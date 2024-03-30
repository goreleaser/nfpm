/*
Copyright 2019 Torsten Curdt

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package apk implements nfpm.Packager providing .apk bindings.
package apk

// Initial implementation from https://gist.github.com/tcurdt/512beaac7e9c12dcf5b6b7603b09d0d8

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/mail"
	"os"
	"strings"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/maps"
	"github.com/goreleaser/nfpm/v2/internal/sign"
	gzip "github.com/klauspost/pgzip"
)

const packagerName = "apk"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// https://wiki.alpinelinux.org/wiki/Architecture
// nolint: gochecknoglobals
var archToAlpine = map[string]string{
	"386":     "x86",
	"amd64":   "x86_64",
	"arm64":   "aarch64",
	"arm6":    "armhf",
	"arm7":    "armv7",
	"ppc64le": "ppc64le",
	"s390":    "s390x",
}

func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	if info.APK.Arch != "" {
		info.Arch = info.APK.Arch
	} else if arch, ok := archToAlpine[info.Arch]; ok {
		info.Arch = arch
	}

	return info
}

// Default apk packager.
// nolint: gochecknoglobals
var Default = &Apk{}

// Apk is an apk packager implementation.
type Apk struct{}

func (a *Apk) ConventionalFileName(info *nfpm.Info) string {
	info = ensureValidArch(info)
	version := pkgver(info)
	return fmt.Sprintf("%s_%s_%s.apk", info.Name, version, info.Arch)
}

// ConventionalExtension returns the file name conventionally used for Apk packages
func (*Apk) ConventionalExtension() string {
	return ".apk"
}

// Package writes a new apk package to the given writer using the given info.
func (*Apk) Package(info *nfpm.Info, apk io.Writer) (err error) {
	if info.Platform != "linux" {
		return fmt.Errorf("invalid platform: %s", info.Platform)
	}
	info = ensureValidArch(info)

	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}

	var bufData bytes.Buffer

	size := int64(0)
	// create the data tgz
	dataDigest, err := createData(&bufData, info, &size)
	if err != nil {
		return err
	}

	// create the control tgz
	var bufControl bytes.Buffer
	controlDigest, err := createControl(&bufControl, info, size, dataDigest)
	if err != nil {
		return err
	}

	if info.APK.Signature.KeyFile == "" && info.APK.Signature.SignFn == nil {
		return combineToApk(apk, &bufControl, &bufData)
	}

	// create the signature tgz
	var bufSignature bytes.Buffer
	if err = createSignature(&bufSignature, info, controlDigest); err != nil {
		return err
	}

	return combineToApk(apk, &bufSignature, &bufControl, &bufData)
}

type writerCounter struct {
	io.Writer
	count  uint64
	writer io.Writer
}

func newWriterCounter(w io.Writer) *writerCounter {
	return &writerCounter{
		writer: w,
	}
}

func (counter *writerCounter) Write(buf []byte) (int, error) {
	n, err := counter.writer.Write(buf)
	atomic.AddUint64(&counter.count, uint64(n))
	return n, err
}

func (counter *writerCounter) Count() uint64 {
	return atomic.LoadUint64(&counter.count)
}

func writeFile(tw *tar.Writer, header *tar.Header, file io.Reader) error {
	header.Format = tar.FormatUSTAR
	header.ChangeTime = time.Time{}
	header.AccessTime = time.Time{}

	err := tw.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}

type tarKind int

const (
	tarFull tarKind = iota
	tarCut
)

func writeTgz(w io.Writer, kind tarKind, builder func(tw *tar.Writer) error, digest hash.Hash) ([]byte, error) {
	mw := io.MultiWriter(digest, w)
	gw := gzip.NewWriter(mw)
	cw := newWriterCounter(gw)
	bw := bufio.NewWriterSize(cw, 4096)
	tw := tar.NewWriter(bw)

	err := builder(tw)
	if err != nil {
		return nil, err
	}

	// handle the cut vs full tars
	// TODO: document this better, why do we need to call bw.Flush twice if it is a full tar vs the cut tar?
	if err = bw.Flush(); err != nil {
		return nil, err
	}
	if err = tw.Close(); err != nil {
		return nil, err
	}
	if kind == tarFull {
		if err = bw.Flush(); err != nil {
			return nil, err
		}
	}

	size := cw.Count()
	alignedSize := (size + 511) & ^uint64(511)

	increase := alignedSize - size
	if increase > 0 {
		b := make([]byte, increase)
		_, err = cw.Write(b)
		if err != nil {
			return nil, err
		}
	}

	if err = gw.Close(); err != nil {
		return nil, err
	}

	return digest.Sum(nil), nil
}

func createData(dataTgz io.Writer, info *nfpm.Info, sizep *int64) ([]byte, error) {
	builderData := createBuilderData(info, sizep)
	dataDigest, err := writeTgz(dataTgz, tarFull, builderData, sha256.New())
	if err != nil {
		return nil, err
	}
	return dataDigest, nil
}

func createControl(controlTgz io.Writer, info *nfpm.Info, size int64, dataDigest []byte) ([]byte, error) {
	builderControl := createBuilderControl(info, size, dataDigest)
	controlDigest, err := writeTgz(controlTgz, tarCut, builderControl, sha1.New()) // nolint:gosec
	if err != nil {
		return nil, err
	}
	return controlDigest, nil
}

func createSignature(signatureTgz io.Writer, info *nfpm.Info, controlSHA1Digest []byte) error {
	signatureBuilder := createSignatureBuilder(controlSHA1Digest, info)
	// we don't actually need to produce a digest here, but writeTgz
	// requires it so we just use SHA1 since it is already imported
	_, err := writeTgz(signatureTgz, tarCut, signatureBuilder, sha1.New()) // nolint:gosec
	if err != nil {
		return &nfpm.ErrSigningFailure{Err: err}
	}

	return nil
}

var errNoKeyAddress = errors.New("key name not set and maintainer mail address empty")

func createSignatureBuilder(digest []byte, info *nfpm.Info) func(*tar.Writer) error {
	return func(tw *tar.Writer) error {
		var signature []byte
		var err error
		if signFn := info.APK.Signature.SignFn; signFn != nil {
			signature, err = signFn(bytes.NewReader(digest))
		} else {
			signature, err = sign.RSASignSHA1Digest(digest,
				info.APK.Signature.KeyFile, info.APK.Signature.KeyPassphrase)
		}
		if err != nil {
			return err
		}

		// needs to exist on the machine during installation: /etc/apk/keys/<keyname>.rsa.pub
		keyname := info.APK.Signature.KeyName
		if keyname == "" {
			addr, err := mail.ParseAddress(info.Maintainer)
			if err != nil {
				return fmt.Errorf("key name not set and unable to parse maintainer mail address: %w", err)
			} else if addr.Address == "" {
				return errNoKeyAddress
			}

			keyname = addr.Address
		}
		if !strings.HasSuffix(keyname, ".rsa.pub") {
			keyname += ".rsa.pub"
		}

		// In principle apk supports RSA signatures over SHA256/512 keys, but in
		// practice verification works but installation segfaults. If this is
		// fixed at some point we should also upgrade the hash. In this case,
		// the file name will have to start with .SIGN.RSA256 or .SIGN.RSA512.
		signHeader := &tar.Header{
			Name: fmt.Sprintf(".SIGN.RSA.%s", keyname),
			Mode: 0o600,
			Size: int64(len(signature)),
		}

		return writeFile(tw, signHeader, bytes.NewReader(signature))
	}
}

func combineToApk(target io.Writer, readers ...io.Reader) error {
	for _, tgz := range readers {
		if _, err := io.Copy(target, tgz); err != nil {
			return err
		}
	}
	return nil
}

func createBuilderControl(info *nfpm.Info, size int64, dataDigest []byte) func(tw *tar.Writer) error {
	return func(tw *tar.Writer) error {
		var infoBuf bytes.Buffer
		if err := writeControl(&infoBuf, controlData{
			Info:          info,
			InstalledSize: size,
			Datahash:      hex.EncodeToString(dataDigest),
		}); err != nil {
			return err
		}
		infoContent := infoBuf.String()

		infoHeader := &tar.Header{
			Name: ".PKGINFO",
			Mode: 0o600,
			Size: int64(len(infoContent)),
		}

		if err := writeFile(tw, infoHeader, strings.NewReader(infoContent)); err != nil {
			return err
		}

		// NOTE: Apk scripts tend to follow the pattern:
		// #!/bin/sh
		//
		// bin/echo 'running preinstall.sh' // do stuff here
		//
		// exit 0
		scripts := map[string]string{
			".pre-install":    info.Scripts.PreInstall,
			".pre-upgrade":    info.APK.Scripts.PreUpgrade,
			".post-install":   info.Scripts.PostInstall,
			".post-upgrade":   info.APK.Scripts.PostUpgrade,
			".pre-deinstall":  info.Scripts.PreRemove,
			".post-deinstall": info.Scripts.PostRemove,
		}
		for _, name := range maps.Keys(scripts) {
			path := scripts[name]
			if path == "" {
				continue
			}
			if err := newScriptInsideTarGz(tw, path, name); err != nil {
				return err
			}
		}

		return nil
	}
}

func newScriptInsideTarGz(out *tar.Writer, path, dest string) error {
	file, err := os.Stat(path) //nolint:gosec
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return newItemInsideTarGz(out, content, &tar.Header{
		Name:     files.ToNixPath(dest),
		Size:     int64(len(content)),
		Mode:     0o755,
		ModTime:  file.ModTime(),
		Typeflag: tar.TypeReg,
	})
}

func newItemInsideTarGz(out *tar.Writer, content []byte, header *tar.Header) error {
	header.Format = tar.FormatPAX
	header.PAXRecords = make(map[string]string)

	hasher := sha1.New()
	_, err := hasher.Write(content)
	if err != nil {
		return fmt.Errorf("failed to hash content of file %s: %w", header.Name, err)
	}
	header.PAXRecords["APK-TOOLS.checksum.SHA1"] = fmt.Sprintf("%x", hasher.Sum(nil))
	if err := out.WriteHeader(header); err != nil {
		return fmt.Errorf("cannot write header of %s file to apk: %w", header.Name, err)
	}
	if _, err := out.Write(content); err != nil {
		return fmt.Errorf("cannot write %s file to apk: %w", header.Name, err)
	}
	return nil
}

func createBuilderData(info *nfpm.Info, sizep *int64) func(tw *tar.Writer) error {
	return func(tw *tar.Writer) error {
		return createFilesInsideTarGz(info, tw, sizep)
	}
}

func createFilesInsideTarGz(info *nfpm.Info, tw *tar.Writer, sizep *int64) (err error) {
	for _, file := range info.Contents {
		file.Destination = files.AsRelativePath(file.Destination)

		switch file.Type {
		case files.TypeDir, files.TypeImplicitDir:
			err = tw.WriteHeader(&tar.Header{
				Name:     file.Destination,
				Mode:     int64(file.FileInfo.Mode),
				Typeflag: tar.TypeDir,
				Uname:    file.FileInfo.Owner,
				Gname:    file.FileInfo.Group,
				ModTime:  file.FileInfo.MTime,
			})
		case files.TypeSymlink:
			err = newItemInsideTarGz(tw, []byte{}, &tar.Header{
				Name:     file.Destination,
				Linkname: file.Source,
				Typeflag: tar.TypeSymlink,
				ModTime:  file.FileInfo.MTime,
			})
		default:
			err = copyToTarAndDigest(file, tw, sizep)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func copyToTarAndDigest(file *files.Content, tw *tar.Writer, sizep *int64) error {
	contents, err := os.ReadFile(file.Source)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(file, file.Source)
	if err != nil {
		return err
	}

	// tar.FileInfoHeader only uses file.Mode().Perm() which masks the mode with
	// 0o777 which we don't want because we want to be able to set the suid bit.
	header.Mode = int64(file.Mode())
	header.Name = files.AsRelativePath(file.Destination)
	header.Uname = file.FileInfo.Owner
	header.Gname = file.FileInfo.Group
	if err = newItemInsideTarGz(tw, contents, header); err != nil {
		return err
	}

	*sizep += file.Size()
	return nil
}

// reference: https://wiki.adelielinux.org/wiki/APK_internals#.PKGINFO
const controlTemplate = `
{{- /* Mandatory fields */ -}}
pkgname = {{.Info.Name}}
pkgver = {{ pkgver .Info }}
arch = {{.Info.Arch}}
size = {{.InstalledSize}}
pkgdesc = {{multiline .Info.Description}}
{{- if .Info.Homepage}}
url = {{.Info.Homepage}}
{{- end }}
{{- if .Info.Maintainer}}
maintainer = {{.Info.Maintainer}}
{{- end }}
{{- range $repl := .Info.Replaces}}
replaces = {{ $repl }}
{{- end }}
{{- range $prov := .Info.Provides}}
provides = {{ $prov }}
{{- end }}
{{- range $dep := .Info.Depends}}
depend = {{ $dep }}
{{- end }}
{{- if .Info.License}}
license = {{.Info.License}}
{{- end }}
datahash = {{.Datahash}}
`

type controlData struct {
	Info          *nfpm.Info
	InstalledSize int64
	Datahash      string
}

func writeControl(w io.Writer, data controlData) error {
	tmpl := template.New("control")
	tmpl.Funcs(template.FuncMap{
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n  ")
			return strings.Trim(ret, " \n")
		},
		"pkgver": pkgver,
	})
	return template.Must(tmpl.Parse(controlTemplate)).Execute(w, data)
}

func pkgver(info *nfpm.Info) string {
	version := info.Version

	if info.Prerelease != "" {
		version += "_" + info.Prerelease
	}

	if rel := info.Release; rel != "" {
		if !strings.HasPrefix(rel, "r") {
			rel = "r" + rel
		}
		version += "-" + rel
	}
	if meta := info.VersionMetadata; meta != "" {
		if !strings.HasPrefix(meta, "p") &&
			!strings.HasPrefix(meta, "cvs") &&
			!strings.HasPrefix(meta, "svn") &&
			!strings.HasPrefix(meta, "git") &&
			!strings.HasPrefix(meta, "hg") {
			meta = "p" + meta
		}
		version += "-" + meta
	}
	return version
}
