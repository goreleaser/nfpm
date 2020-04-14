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

// Package apk (someday) implements nfpm.Packager providing .apk bindings.
package apk

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/goreleaser/nfpm/glob"

	"github.com/pkg/errors"

	"github.com/goreleaser/nfpm"
)

// nolint: gochecknoinits
func init() {
	nfpm.Register("apk", Default)
}

// nolint: gochecknoglobals
var archToAlpine = map[string]string{
	"386":   "x86",
	"amd64": "x86_64",

	"arm":   "armhf",
	"arm6":  "armhf",
	"arm7":  "armhf",
	"arm64": "aarch64",

	// "s390x":  "???",
}

// Default apk packager
// nolint: gochecknoglobals
var Default = &Apk{}

// Apk is a apk packager implementation
type Apk struct{}

// Package writes a new apk package to the given writer using the given info
func (*Apk) Package(info *nfpm.Info, apk io.Writer) (err error) {
	arch, ok := archToAlpine[info.Arch]
	if ok {
		info.Arch = arch
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
	controlDigest, err := createControl(&bufControl, info, dataDigest, size)
	if err != nil {
		return err
	}

	var bufSignature bytes.Buffer
	err = createSignature(&bufSignature, controlDigest, info)
	if err != nil {
		return err
	}

	// combine
	return combineToApk(apk, &bufData, &bufControl, &bufSignature)
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
	// if hash {
	// 	header.Format = tar.FormatPAX

	// 	digest := sha1.New()
	// 	_, err := io.Copy(digest, file)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	header.PAXRecords = map[string]string{
	// 		"APK-TOOLS.checksum.SHA1": "f572d396fae9206628714fb2ce00f72e94f2258f",
	// 		// "APK-TOOLS.checksum.SHA1": hex.EncodeToString(digest.Sum(nil)),
	// 	}
	// } else {
	header.Format = tar.FormatUSTAR
	header.ChangeTime = time.Time{}
	header.AccessTime = time.Time{}
	// }

	err := tw.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}

	return nil
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

func parseRsaPrivateKeyFromPemStr(privPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return priv, nil
}

func fileToBase64String(file string) (string, error) {
	fileBytes, err := ioutil.ReadFile(filepath.Clean(file))
	if err != nil {
		return "", err
	}
	base64Contents := base64.StdEncoding.EncodeToString(fileBytes)
	return base64Contents, nil
}

func createSignature(signatureTgz io.Writer, controlDigest []byte, info *nfpm.Info) error {
	var pemBytes []byte
	var err error
	switch {
	case info.PrivateKeyBase64 != "":
		if pemBytes, err = base64.StdEncoding.DecodeString(info.PrivateKeyBase64); err != nil {
			return err
		}
	case info.PrivateKeyFile != "":
		if pemBytes, err = ioutil.ReadFile(filepath.Clean(info.PrivateKeyFile)); err != nil {
			return err
		}
	default:
		fmt.Printf("yaml configuration of 'privatekeybase64' or 'privatekeyfile' is required for .apk\n")
	}
	priv, err := parseRsaPrivateKeyFromPemStr(string(pemBytes))
	if err != nil {
		return err
	}
	signed, err := priv.Sign(rand.Reader, controlDigest, crypto.SHA256)
	if err != nil {
		return err
	}

	// create the signature tgz
	builderSignature := createBuilderSignature(signed, info)
	_, err = writeTgz(signatureTgz, tarCut, builderSignature, sha256.New())
	return err
}

func createData(dataTgz io.Writer, info *nfpm.Info, sizep *int64) ([]byte, error) {
	builderData := createBuilderData(info, sizep)
	dataDigest, err := writeTgz(dataTgz, tarFull, builderData, sha256.New())
	if err != nil {
		return nil, err
	}
	return dataDigest, nil
}

func createControl(controlTgz io.Writer, info *nfpm.Info, dataDigest []byte, size int64) ([]byte, error) {
	builderControl := createBuilderControl(info, size, dataDigest)
	controlDigest, err := writeTgz(controlTgz, tarCut, builderControl, sha256.New())
	if err != nil {
		return nil, err
	}
	return controlDigest, nil
}

func combineToApk(target io.Writer, dataTgz, controlTgz, signatureTgz io.Reader) (err error) {
	tgzs := []io.Reader{signatureTgz, controlTgz, dataTgz}

	for _, tgz := range tgzs {
		if _, err = io.Copy(target, tgz); err != nil {
			return err
		}
	}

	return err
}

func createBuilderSignature(signed []byte, info *nfpm.Info) func(tw *tar.Writer) error {
	return func(tw *tar.Writer) error {
		keyname := info.KeyName
		// needs to exist on the machine: /etc/apk/keys/<keyname>.rsa.pub

		signContent := signed
		signHeader := &tar.Header{
			Name: fmt.Sprintf(".SIGN.RSA.%s.rsa.pub", keyname),
			Mode: 0600,
			Size: int64(len(signContent)),
		}

		if err := writeFile(tw, signHeader, bytes.NewReader(signContent)); err != nil {
			return err
		}

		return nil
	}
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
			Mode: 0600,
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
		for script, dest := range map[string]string{
			info.Scripts.PreInstall:  ".pre-install",
			info.Scripts.PostInstall: ".post-install",
			info.Scripts.PreRemove:   ".pre-deinstall",
			info.Scripts.PostRemove:  ".post-deinstall",
		} {
			if script != "" {
				if err := newScriptInsideTarGz(tw, script, dest); err != nil {
					return err
				}
			}
		}

		return nil
	}
}

func newScriptInsideTarGz(out *tar.Writer, path, dest string) error {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	return newItemInsideTarGz(out, content, &tar.Header{
		Name:     filepath.ToSlash(dest),
		Size:     int64(len(content)),
		Mode:     0755,
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
		Format:   tar.FormatGNU,
	})
}

func newItemInsideTarGz(out *tar.Writer, content []byte, header *tar.Header) error {
	if err := out.WriteHeader(header); err != nil {
		return errors.Wrapf(err, "cannot write header of %s file to control.tar.gz", header.Name)
	}
	if _, err := out.Write(content); err != nil {
		return errors.Wrapf(err, "cannot write %s file to control.tar.gz", header.Name)
	}
	return nil
}

func createBuilderData(info *nfpm.Info, sizep *int64) func(tw *tar.Writer) error {
	var created = map[string]bool{}

	return func(tw *tar.Writer) error {
		// handle empty folders
		if err := createEmptyFoldersInsideTarGz(info, tw, created); err != nil {
			return err
		}

		// handle Files and ConfigFiles
		return createFilesInsideTarGz(info, tw, created, sizep)
	}
}

func createFilesInsideTarGz(info *nfpm.Info, tw *tar.Writer, created map[string]bool, sizep *int64) error {
	for _, files := range []map[string]string{
		info.Files,
		info.ConfigFiles,
	} {
		for srcglob, dstroot := range files {
			globbed, err := glob.Glob(srcglob, dstroot)
			if err != nil {
				return err
			}
			for src, dst := range globbed {
				// when used as a lib, target may not be set.
				// in that case, src will always have the empty sufix, and all
				// files will be ignored.
				if info.Target != "" && strings.HasSuffix(src, info.Target) {
					fmt.Printf("skipping %s because it has the suffix %s", src, info.Target)
					continue
				}
				if err := createTree(tw, dst, created); err != nil {
					return err
				}

				// copied from deb.go->copyToTarAndDigest()
				err := copyToTarAndDigest(src, dst, tw, sizep, created)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copyToTarAndDigest(src, dst string, tw *tar.Writer, sizep *int64, created map[string]bool) error {
	file, err := os.OpenFile(src, os.O_RDONLY, 0600) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, "could not add file to the archive")
	}
	// don't care if it errs while closing...
	defer file.Close() // nolint: errcheck
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		// TODO: this should probably return an error
		return nil
	}
	/*					var header = tar.Header{
							Name:    filepath.ToSlash(dst[1:]),
							Size:    info.Size(),
							Mode:    int64(info.Mode()),
							ModTime: time.Now(),
							Format:  tar.FormatGNU,
						}
	*/
	header, err := tar.FileInfoHeader(info, src)
	if err != nil {
		log.Print(err)
		return err
	}
	header.Name = filepath.ToSlash(dst[1:])

	info, err = file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		// TODO: this should probably return an error
		return nil
	}

	err = writeFile(tw, header, file)
	if err != nil {
		return err
	}

	*sizep += info.Size()
	created[src] = true
	return nil
}

func createEmptyFoldersInsideTarGz(info *nfpm.Info, out *tar.Writer, created map[string]bool) error {
	for _, folder := range info.EmptyFolders {
		// this .nope is actually not created, because createTree ignore the
		// last part of the path, assuming it is a file.
		// TODO: should probably refactor this
		if err := createTree(out, filepath.Join(folder, ".nope"), created); err != nil {
			return err
		}
	}
	return nil
}

// this is needed because the data.tar.gz file should have the empty folders
// as well, so we walk through the dst and create all subfolders.
func createTree(tarw *tar.Writer, dst string, created map[string]bool) error {
	for _, path := range pathsToCreate(dst) {
		if created[path] {
			// skipping dir that was previously created inside the archive
			// (eg: usr/)
			continue
		}
		if err := tarw.WriteHeader(&tar.Header{
			Name:     filepath.ToSlash(path + "/"),
			Mode:     0755,
			Typeflag: tar.TypeDir,
			Format:   tar.FormatGNU,
			ModTime:  time.Now(),
		}); err != nil {
			return errors.Wrap(err, "failed to create folder")
		}
		created[path] = true
	}
	return nil
}

func pathsToCreate(dst string) []string {
	var paths []string
	var base = dst[1:]
	for {
		base = filepath.Dir(base)
		if base == "." {
			break
		}
		paths = append(paths, base)
	}
	// we don't really need to create those things in order apparently, but,
	// it looks really weird if we don't.
	var result []string
	for i := len(paths) - 1; i >= 0; i-- {
		result = append(result, paths[i])
	}
	return result
}

const controlTemplate = `
{{- /* Mandatory fields */ -}}
pkgname = {{.Info.Name}}
pkgver = {{ if .Info.Epoch}}{{ .Info.Epoch }}:{{ end }}{{.Info.Version}}
         {{- if .Info.Release}}-{{ .Info.Release }}{{- end }}
         {{- if .Info.Prerelease}}~{{ .Info.Prerelease }}{{- end }}
         {{- if .Info.Deb.VersionMetadata}}+{{ .Info.Deb.VersionMetadata }}{{- end }}
arch = {{.Info.Arch}}
size = {{.InstalledSize}}
pkgdesc = {{multiline .Info.Description}}
{{- if .Info.Homepage}}
url = {{.Info.Homepage}}
{{- end }}
{{- if .Info.Maintainer}}
maintainer = {{.Info.Maintainer}}
{{- end }}
{{- with .Info.Replaces}}
replaces = {{join .}}
{{- end }}
{{- with .Info.Provides}}
provides = {{join .}}
{{- end }}
{{- with .Info.Depends}}
depend = {{join .}}
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
	var tmpl = template.New("control")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n  ")
			return strings.Trim(ret, " \n")
		},
	})
	return template.Must(tmpl.Parse(controlTemplate)).Execute(w, data)
}
