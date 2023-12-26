// Package arch implements nfpm.Packager providing bindings for Arch Linux packages.
package arch

import (
	"archive/tar"
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/maps"
	"github.com/goreleaser/nfpm/v2/internal/modtime"
	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/pgzip"
)

var ErrInvalidPkgName = errors.New("archlinux: package names may only contain alphanumeric characters or one of ., _, +, or -, and may not start with hyphen or dot")

const packagerName = "archlinux"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// Default ArchLinux packager.
// nolint: gochecknoglobals
var Default = ArchLinux{}

// ArchLinux packager.
// nolint: revive
type ArchLinux struct{}

// nolint: gochecknoglobals
var archToArchLinux = map[string]string{
	"all":   "any",
	"amd64": "x86_64",
	"386":   "i686",
	"arm64": "aarch64",
	"arm7":  "armv7h",
	"arm6":  "armv6h",
	"arm5":  "arm",
}

func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	if info.ArchLinux.Arch != "" {
		info.Arch = info.ArchLinux.Arch
	} else if arch, ok := archToArchLinux[info.Arch]; ok {
		info.Arch = arch
	}

	return info
}

// ConventionalFileName returns a file name for a package conforming
// to Arch Linux package naming guidelines. See:
// https://wiki.archlinux.org/title/Arch_package_guidelines#Package_naming
func (ArchLinux) ConventionalFileName(info *nfpm.Info) string {
	info = ensureValidArch(info)

	pkgrel, err := strconv.Atoi(info.Release)
	if err != nil {
		pkgrel = 1
	}

	name := fmt.Sprintf(
		"%s-%s-%d-%s.pkg.tar.zst",
		info.Name,
		info.Version+strings.ReplaceAll(info.Prerelease, "-", "_"),
		pkgrel,
		info.Arch,
	)

	return validPkgName(name)
}

// validPkgName removes any invalid characters from a string
func validPkgName(s string) string {
	s = strings.Map(mapValidChar, s)
	s = strings.TrimLeft(s, "-.")
	return s
}

// nameIsValid checks whether a package name is valid
func nameIsValid(s string) bool {
	return s != "" && s == validPkgName(s)
}

// mapValidChar returns r if it is allowed, otherwise, returns -1
func mapValidChar(r rune) rune {
	if r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		isOneOf(r, '.', '_', '+', '-') {
		return r
	}
	return -1
}

// isOneOf checks whether a rune is one of the runes in rr
func isOneOf(r rune, rr ...rune) bool {
	for _, char := range rr {
		if r == char {
			return true
		}
	}
	return false
}

// Package writes a new archlinux package to the given writer using the given info.
func (ArchLinux) Package(info *nfpm.Info, w io.Writer) error {
	if info.Platform != "linux" {
		return fmt.Errorf("invalid platform: %s", info.Platform)
	}
	info = ensureValidArch(info)

	err := nfpm.PrepareForPackager(info, packagerName)
	if err != nil {
		return err
	}

	if !nameIsValid(info.Name) {
		return ErrInvalidPkgName
	}

	zw, err := zstd.NewWriter(w)
	if err != nil {
		return err
	}
	defer zw.Close()

	tw := tar.NewWriter(zw)
	defer tw.Close()

	entries, totalSize, err := createFilesInTar(info, tw)
	if err != nil {
		return fmt.Errorf("create files in tar: %w", err)
	}

	pkginfoEntry, err := createPkginfo(info, tw, totalSize)
	if err != nil {
		return fmt.Errorf("create pkg info: %w", err)
	}

	// .PKGINFO must be the first entry in .MTREE
	entries = append([]MtreeEntry{*pkginfoEntry}, entries...)

	err = createMtree(tw, entries, modtime.Get(info.MTime))
	if err != nil {
		return fmt.Errorf("create mtree: %w", err)
	}

	return createScripts(info, tw)
}

// ConventionalExtension returns the file name conventionally used for Arch Linux packages
func (ArchLinux) ConventionalExtension() string {
	return ".pkg.tar.zst"
}

// createFilesInTar adds the files described in the given info to the given tar writer
func createFilesInTar(info *nfpm.Info, tw *tar.Writer) ([]MtreeEntry, int64, error) {
	entries := make([]MtreeEntry, 0, len(info.Contents))
	var totalSize int64

	for _, content := range info.Contents {
		content.Destination = files.AsRelativePath(content.Destination)

		switch content.Type {
		case files.TypeDir, files.TypeImplicitDir:
			entries = append(entries, MtreeEntry{
				Destination: content.Destination,
				Time:        content.ModTime().Unix(),
				Mode:        int64(content.Mode()),
				Type:        files.TypeDir,
			})

			if err := tw.WriteHeader(&tar.Header{
				Name:     content.Destination,
				Mode:     int64(content.Mode()),
				Typeflag: tar.TypeDir,
				ModTime:  content.ModTime(),
				Uname:    content.FileInfo.Owner,
				Gname:    content.FileInfo.Group,
			}); err != nil {
				return nil, 0, err
			}
		case files.TypeSymlink:
			if err := tw.WriteHeader(&tar.Header{
				Name:     content.Destination,
				Linkname: content.Source,
				ModTime:  content.ModTime(),
				Typeflag: tar.TypeSymlink,
			}); err != nil {
				return nil, 0, err
			}

			entries = append(entries, MtreeEntry{
				LinkSource:  content.Source,
				Destination: content.Destination,
				Time:        content.ModTime().Unix(),
				Mode:        0o777,
				Type:        content.Type,
			})
		default:
			src, err := os.Open(content.Source)
			if err != nil {
				return nil, 0, err
			}

			header := &tar.Header{
				Name:     content.Destination,
				Mode:     int64(content.Mode()),
				Typeflag: tar.TypeReg,
				Size:     content.Size(),
				ModTime:  content.ModTime(),
			}

			if content.FileInfo != nil && content.Mode() != 0 {
				header.Mode = int64(content.Mode())
			}

			if content.FileInfo != nil && !content.ModTime().IsZero() {
				header.ModTime = content.ModTime()
			}

			if content.FileInfo != nil && content.Size() != 0 {
				header.Size = content.Size()
			}

			err = tw.WriteHeader(header)
			if err != nil {
				return nil, 0, err
			}

			sha256Hash := sha256.New()
			md5Hash := md5.New()

			w := io.MultiWriter(tw, sha256Hash, md5Hash)

			_, err = io.Copy(w, src)
			if err != nil {
				return nil, 0, err
			}

			entries = append(entries, MtreeEntry{
				Destination: content.Destination,
				Time:        content.ModTime().Unix(),
				Mode:        int64(content.Mode()),
				Size:        content.Size(),
				Type:        content.Type,
				MD5:         md5Hash.Sum(nil),
				SHA256:      sha256Hash.Sum(nil),
			})

			totalSize += content.Size()
		}
	}

	return entries, totalSize, nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func createPkginfo(info *nfpm.Info, tw *tar.Writer, totalSize int64) (*MtreeEntry, error) {
	if !nameIsValid(info.Name) {
		return nil, ErrInvalidPkgName
	}

	buf := &bytes.Buffer{}

	info = ensureValidArch(info)

	pkgrel, err := strconv.Atoi(info.Release)
	if err != nil {
		pkgrel = 1
	}

	pkgver := fmt.Sprintf("%s-%d", info.Version, pkgrel)
	if info.Epoch != "" {
		epoch, err := strconv.ParseUint(info.Epoch, 10, 64)
		if err == nil {
			pkgver = fmt.Sprintf(
				"%d:%s%s-%d",
				epoch,
				info.Version,
				strings.ReplaceAll(info.Prerelease, "-", "_"),
				pkgrel,
			)
		}
	}

	// Description cannot contain newlines
	pkgdesc := strings.ReplaceAll(info.Description, "\n", " ")

	_, err = io.WriteString(buf, "# Generated by nfpm\n")
	if err != nil {
		return nil, err
	}

	builddate := strconv.FormatInt(modtime.Get(info.MTime).Unix(), 10)
	totalSizeStr := strconv.FormatInt(totalSize, 10)

	err = writeKVPairs(buf, map[string]string{
		"size":      totalSizeStr,
		"pkgname":   info.Name,
		"pkgbase":   defaultStr(info.ArchLinux.Pkgbase, info.Name),
		"pkgver":    pkgver,
		"pkgdesc":   pkgdesc,
		"url":       info.Homepage,
		"builddate": builddate,
		"packager":  defaultStr(info.ArchLinux.Packager, "Unknown Packager"),
		"arch":      info.Arch,
		"license":   info.License,
	})
	if err != nil {
		return nil, err
	}

	for _, replaces := range info.Replaces {
		err = writeKVPair(buf, "replaces", replaces)
		if err != nil {
			return nil, err
		}
	}

	for _, conflict := range info.Conflicts {
		err = writeKVPair(buf, "conflict", conflict)
		if err != nil {
			return nil, err
		}
	}

	for _, provides := range info.Provides {
		err = writeKVPair(buf, "provides", provides)
		if err != nil {
			return nil, err
		}
	}

	for _, depend := range info.Depends {
		err = writeKVPair(buf, "depend", depend)
		if err != nil {
			return nil, err
		}
	}

	for _, content := range info.Contents {
		if content.Type == files.TypeConfig || content.Type == files.TypeConfigNoReplace {
			path := files.AsRelativePath(content.Destination)

			if err := writeKVPair(buf, "backup", path); err != nil {
				return nil, err
			}
		}
	}

	size := buf.Len()

	err = tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Name:     ".PKGINFO",
		Size:     int64(size),
		ModTime:  modtime.Get(info.MTime),
	})
	if err != nil {
		return nil, err
	}

	md5Hash := md5.New()
	sha256Hash := sha256.New()

	r := io.TeeReader(buf, md5Hash)
	r = io.TeeReader(r, sha256Hash)

	if _, err = io.Copy(tw, r); err != nil {
		return nil, err
	}

	return &MtreeEntry{
		Destination: ".PKGINFO",
		Time:        modtime.Get(info.MTime).Unix(),
		Mode:        0o644,
		Size:        int64(size),
		Type:        files.TypeFile,
		MD5:         md5Hash.Sum(nil),
		SHA256:      sha256Hash.Sum(nil),
	}, nil
}

func writeKVPairs(w io.Writer, pairs map[string]string) error {
	for _, key := range maps.Keys(pairs) {
		if err := writeKVPair(w, key, pairs[key]); err != nil {
			return err
		}
	}
	return nil
}

func writeKVPair(w io.Writer, key, value string) error {
	if value == "" {
		return nil
	}

	_, err := io.WriteString(w, key)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, " = ")
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, value)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, "\n")
	return err
}

type MtreeEntry struct {
	LinkSource  string
	Destination string
	Time        int64
	Mode        int64
	Size        int64
	Type        string
	MD5         []byte
	SHA256      []byte
}

func (me *MtreeEntry) WriteTo(w io.Writer) (int64, error) {
	switch me.Type {
	case files.TypeDir, files.TypeImplicitDir:
		n, err := fmt.Fprintf(
			w,
			"./%s time=%d.0 mode=%o type=dir\n",
			me.Destination,
			me.Time,
			me.Mode,
		)
		return int64(n), err
	case files.TypeSymlink:
		n, err := fmt.Fprintf(
			w,
			"./%s time=%d.0 mode=%o type=link link=%s\n",
			me.Destination,
			me.Time,
			me.Mode,
			me.LinkSource,
		)
		return int64(n), err
	default:
		n, err := fmt.Fprintf(
			w,
			"./%s time=%d.0 mode=%o size=%d type=file md5digest=%x sha256digest=%x\n",
			me.Destination,
			me.Time,
			me.Mode,
			me.Size,
			me.MD5,
			me.SHA256,
		)
		return int64(n), err
	}
}

func createMtree(tw *tar.Writer, entries []MtreeEntry, mtime time.Time) error {
	buf := &bytes.Buffer{}
	gw := pgzip.NewWriter(buf)
	defer gw.Close()

	_, err := io.WriteString(gw, "#mtree\n")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		_, err = entry.WriteTo(gw)
		if err != nil {
			return err
		}
	}

	gw.Close()

	err = tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Name:     ".MTREE",
		Size:     int64(buf.Len()),
		ModTime:  mtime,
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, buf)
	return err
}

func createScripts(info *nfpm.Info, tw *tar.Writer) error {
	scripts := map[string]string{}

	if info.Scripts.PreInstall != "" {
		scripts["pre_install"] = info.Scripts.PreInstall
	}

	if info.Scripts.PostInstall != "" {
		scripts["post_install"] = info.Scripts.PostInstall
	}

	if info.Scripts.PreRemove != "" {
		scripts["pre_remove"] = info.Scripts.PreRemove
	}

	if info.Scripts.PostRemove != "" {
		scripts["post_remove"] = info.Scripts.PostRemove
	}

	if info.ArchLinux.Scripts.PreUpgrade != "" {
		scripts["pre_upgrade"] = info.ArchLinux.Scripts.PreUpgrade
	}

	if info.ArchLinux.Scripts.PostUpgrade != "" {
		scripts["post_upgrade"] = info.ArchLinux.Scripts.PostUpgrade
	}

	if len(scripts) == 0 {
		return nil
	}

	buf := &bytes.Buffer{}

	err := writeScripts(buf, scripts)
	if err != nil {
		return err
	}

	err = tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Name:     ".INSTALL",
		Size:     int64(buf.Len()),
		ModTime:  modtime.Get(info.MTime),
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, buf)
	return err
}

func writeScripts(w io.Writer, scripts map[string]string) error {
	for _, script := range maps.Keys(scripts) {
		fmt.Fprintf(w, "function %s() {\n", script)

		fl, err := os.Open(scripts[script])
		if err != nil {
			return err
		}

		_, err = io.Copy(w, fl)
		if err != nil {
			return err
		}

		_ = fl.Close()

		_, err = io.WriteString(w, "\n}\n\n")
		if err != nil {
			return err
		}
	}

	return nil
}
