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
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
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
	if err := info.Validate(); err != nil {
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
		return err
	}

	pkginfoEntry, err := createPkginfo(info, tw, totalSize)
	if err != nil {
		return err
	}

	// .PKGINFO must be the first entry in .MTREE
	entries = append([]MtreeEntry{*pkginfoEntry}, entries...)

	err = createMtree(info, tw, entries)
	if err != nil {
		return err
	}

	return createScripts(info, tw)
}

// ConventionalExtension returns the file name conventionally used for Arch Linux packages
func (ArchLinux) ConventionalExtension() string {
	return ".pkg.tar.zst"
}

// createFilesInTar adds the files described in the given info to the given tar writer
func createFilesInTar(info *nfpm.Info, tw *tar.Writer) ([]MtreeEntry, int64, error) {
	created := map[string]struct{}{}
	var entries []MtreeEntry
	var totalSize int64

	var contents []*files.Content

	for _, content := range info.Contents {
		if content.Packager != "" && content.Packager != packagerName {
			continue
		}

		switch content.Type {
		case "dir", "symlink":
			contents = append(contents, content)
			continue
		}

		fi, err := os.Stat(content.Source)
		if err != nil {
			return nil, 0, err
		}

		if fi.IsDir() {
			err = filepath.WalkDir(content.Source, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				relPath := strings.TrimPrefix(path, content.Source)

				if d.IsDir() {
					return nil
				}

				c := &files.Content{
					Source:      path,
					Destination: filepath.Join(content.Destination, relPath),
					FileInfo: &files.ContentFileInfo{
						Mode: d.Type(),
					},
				}

				if d.Type()&os.ModeSymlink != 0 {
					c.Type = "symlink"
				}

				contents = append(contents, c)
				return nil
			})
			if err != nil {
				return nil, 0, err
			}
		} else {
			contents = append(contents, content)
		}
	}

	for _, content := range contents {
		path := normalizePath(content.Destination)

		switch content.Type {
		case "ghost":
			// Ignore ghost files
		case "dir":
			err := createDirs(content.Destination, tw, created)
			if err != nil {
				return nil, 0, err
			}

			modtime := time.Now()
			// If the time is given, use it
			if content.FileInfo != nil && !content.ModTime().IsZero() {
				modtime = content.ModTime()
			}

			entries = append(entries, MtreeEntry{
				Destination: path,
				Time:        modtime.Unix(),
				Type:        content.Type,
			})
		case "symlink":
			dir := filepath.Dir(path)
			err := createDirs(dir, tw, created)
			if err != nil {
				return nil, 0, err
			}

			modtime := time.Now()
			// If the time is given, use it
			if content.FileInfo != nil && !content.ModTime().IsZero() {
				modtime = content.ModTime()
			}

			err = tw.WriteHeader(&tar.Header{
				Name:     normalizePath(content.Destination),
				Linkname: content.Source,
				ModTime:  modtime,
				Typeflag: tar.TypeSymlink,
			})
			if err != nil {
				return nil, 0, err
			}

			entries = append(entries, MtreeEntry{
				LinkSource:  content.Source,
				Destination: path,
				Time:        modtime.Unix(),
				Mode:        0o777,
				Type:        content.Type,
			})
		default:
			dir := filepath.Dir(path)
			err := createDirs(dir, tw, created)
			if err != nil {
				return nil, 0, err
			}

			src, err := os.Open(content.Source)
			if err != nil {
				return nil, 0, err
			}

			srcFi, err := src.Stat()
			if err != nil {
				return nil, 0, err
			}

			header := &tar.Header{
				Name:     path,
				Mode:     int64(srcFi.Mode()),
				Typeflag: tar.TypeReg,
				Size:     srcFi.Size(),
				ModTime:  srcFi.ModTime(),
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
				Destination: path,
				Time:        srcFi.ModTime().Unix(),
				Mode:        int64(srcFi.Mode()),
				Size:        srcFi.Size(),
				Type:        content.Type,
				MD5:         md5Hash.Sum(nil),
				SHA256:      sha256Hash.Sum(nil),
			})

			totalSize += srcFi.Size()
		}
	}

	return entries, totalSize, nil
}

func createDirs(dst string, tw *tar.Writer, created map[string]struct{}) error {
	for _, path := range neededPaths(dst) {
		path = normalizePath(path) + "/"

		if _, ok := created[path]; ok {
			continue
		}

		err := tw.WriteHeader(&tar.Header{
			Name:     path,
			Mode:     0o755,
			Typeflag: tar.TypeDir,
			ModTime:  time.Now(),
			Uname:    "root",
			Gname:    "root",
		})
		if err != nil {
			return fmt.Errorf("failed to create folder: %w", err)
		}

		created[path] = struct{}{}
	}

	return nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func neededPaths(dst string) []string {
	dst = files.ToNixPath(dst)
	split := strings.Split(strings.Trim(dst, "/."), "/")

	var sb strings.Builder
	var paths []string
	for index, elem := range split {
		if index != 0 {
			sb.WriteRune('/')
		}
		sb.WriteString(elem)
		paths = append(paths, sb.String())
	}

	return paths
}

func normalizePath(src string) string {
	return files.ToNixPath(strings.TrimPrefix(src, "/"))
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

	builddate := strconv.FormatInt(time.Now().Unix(), 10)
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
		if content.Type == "config" || content.Type == "config|noreplace" {
			path := normalizePath(content.Destination)
			path = strings.TrimPrefix(path, "./")

			err = writeKVPair(buf, "backup", path)
			if err != nil {
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
		ModTime:  time.Now(),
	})
	if err != nil {
		return nil, err
	}

	md5Hash := md5.New()
	sha256Hash := sha256.New()

	r := io.TeeReader(buf, md5Hash)
	r = io.TeeReader(r, sha256Hash)

	_, err = io.Copy(tw, r)
	if err != nil {
		return nil, err
	}

	return &MtreeEntry{
		Destination: ".PKGINFO",
		Time:        time.Now().Unix(),
		Mode:        0o644,
		Size:        int64(size),
		Type:        "file",
		MD5:         md5Hash.Sum(nil),
		SHA256:      sha256Hash.Sum(nil),
	}, nil
}

func writeKVPairs(w io.Writer, s map[string]string) error {
	for key, val := range s {
		err := writeKVPair(w, key, val)
		if err != nil {
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
	case "dir":
		n, err := fmt.Fprintf(
			w,
			"./%s time=%d.0 type=dir\n",
			normalizePath(me.Destination),
			me.Time,
		)
		return int64(n), err
	case "symlink":
		n, err := fmt.Fprintf(
			w,
			"./%s time=%d.0 mode=%o type=link link=%s\n",
			normalizePath(me.Destination),
			me.Time,
			me.Mode,
			me.LinkSource,
		)
		return int64(n), err
	default:
		n, err := fmt.Fprintf(
			w,
			"./%s time=%d.0 mode=%o size=%d type=file md5digest=%x sha256digest=%x\n",
			normalizePath(me.Destination),
			me.Time,
			me.Mode,
			me.Size,
			me.MD5,
			me.SHA256,
		)
		return int64(n), err
	}
}

func createMtree(info *nfpm.Info, tw *tar.Writer, entries []MtreeEntry) error {
	buf := &bytes.Buffer{}
	gw := pgzip.NewWriter(buf)
	defer gw.Close()

	created := map[string]struct{}{}

	_, err := io.WriteString(gw, "#mtree\n")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		destDir := filepath.Dir(entry.Destination)

		dirs := createDirsMtree(destDir, created)
		for _, dir := range dirs {
			_, err = dir.WriteTo(gw)
			if err != nil {
				return err
			}
		}

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
		ModTime:  time.Now(),
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, buf)
	return err
}

func createDirsMtree(dst string, created map[string]struct{}) []MtreeEntry {
	var out []MtreeEntry
	for _, path := range neededPaths(dst) {
		path = normalizePath(path) + "/"

		if path == "./" {
			continue
		}

		if _, ok := created[path]; ok {
			continue
		}

		out = append(out, MtreeEntry{
			Destination: path,
			Time:        time.Now().Unix(),
			Mode:        0o755,
			Type:        "dir",
		})

		created[path] = struct{}{}
	}
	return out
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
		ModTime:  time.Now(),
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, buf)
	return err
}

func writeScripts(w io.Writer, scripts map[string]string) error {
	for script, path := range scripts {
		fmt.Fprintf(w, "function %s() {\n", script)

		fl, err := os.Open(path)
		if err != nil {
			return err
		}

		_, err = io.Copy(w, fl)
		if err != nil {
			return err
		}

		fl.Close()

		_, err = io.WriteString(w, "\n}\n\n")
		if err != nil {
			return err
		}
	}

	return nil
}
