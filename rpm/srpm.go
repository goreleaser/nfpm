package rpm

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/modtime"
	"go.digitalxero.dev/rpm"
)

// tagSourcePackage is RPMTAG_SOURCEPACKAGE, set by the library on source
// packages. Kept as a constant for the tests that assert it.
// https://github.com/rpm-software-management/rpm/blob/master/include/rpm/rpmtag.h#L183
const tagSourcePackage = 1106

// packageSRPM builds a source RPM (.src.rpm) and writes it to w.
//
// nfpm has no spec/source concept, so we synthesize a self-contained,
// rebuildable spec from the package metadata and bundle the would-be payload as
// a single source tarball that the spec's %install lays down verbatim. Running
// `rpmbuild --rebuild` on the result reproduces the binary RPM.
func (r *RPM) packageSRPM(info *nfpm.Info, w io.Writer) error {
	sourceName := fmt.Sprintf("%s-%s.tar.gz", info.Name, formatVersion(info))

	tarPath, err := buildPayloadTar(info)
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)

	spec, err := generateSpec(info, sourceName)
	if err != nil {
		return err
	}

	b := rpm.NewSourcePackage()
	if err := applySourceMetadata(b, info); err != nil {
		return err
	}
	b.WithSpecFile(info.Name+".spec", []byte(spec))
	b.WithSourceFromPath(sourceName, tarPath)
	if fn := signFunc(info); fn != nil {
		b.WithPGPSignFunc(fn)
	}

	pkg, err := b.Build()
	if err != nil {
		return err
	}

	_, err = pkg.WriteTo(w)
	return err
}

// applySourceMetadata maps scalar metadata onto the source-package builder. The
// architecture is intentionally left as the library's "src" default; the target
// build architecture is carried by the spec's BuildArch field instead.
func applySourceMetadata(b rpm.SourcePackageBuilder, info *nfpm.Info) error {
	b.WithName(info.Name)
	b.WithVersion(formatVersion(info))
	b.WithRelease(defaultTo(info.Release, "1"))
	b.WithSummary(defaultTo(info.RPM.Summary, strings.Split(info.Description, "\n")[0]))
	b.WithDescription(info.Description)
	if info.Platform != "" {
		b.WithOS(info.Platform)
	}
	b.WithLicense(info.License)
	b.WithURL(info.Homepage)
	b.WithVendor(info.Vendor)
	b.WithGroup(info.RPM.Group)
	b.WithPackager(defaultTo(info.RPM.Packager, info.Maintainer))
	b.WithBuildTime(modtime.Get(info.MTime))

	host, err := buildHost(info)
	if err != nil {
		return err
	}
	b.WithBuildHost(host)

	if epoch, ok, err := parseEpoch(info); err != nil {
		return err
	} else if ok {
		b.WithEpoch(epoch)
	}

	comp, err := parseCompression(info.RPM.Compression)
	if err != nil {
		return err
	}
	b.WithCompressor(comp)

	return nil
}

// buildPayloadTar writes the package payload to a temporary gzip-compressed tar
// file and returns its path. The caller is responsible for removing it.
func buildPayloadTar(info *nfpm.Info) (string, error) {
	tmp, err := os.CreateTemp("", "nfpm-srpm-*.tar.gz")
	if err != nil {
		return "", err
	}
	path := tmp.Name()

	if err := writePayloadTar(tmp, info); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func writePayloadTar(w io.Writer, info *nfpm.Info) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	mtime := modtime.Get(info.MTime)
	for _, content := range info.Contents {
		if content.Packager != "" && content.Packager != contentPackager {
			continue
		}
		if err := addTarEntry(tw, content, mtime); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addTarEntry(tw *tar.Writer, content *files.Content, mtime time.Time) error {
	name := strings.TrimPrefix(files.ToNixPath(content.Destination), "/")
	owner := defaultTo(content.FileInfo.Owner, "root")
	group := defaultTo(content.FileInfo.Group, "root")

	switch content.Type {
	case files.TypeRPMGhost, files.TypeImplicitDir:
		// Ghost files are not shipped; implicit directories are not owned.
		return nil
	case files.TypeSymlink:
		return tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     name,
			Linkname: content.Source,
			Mode:     0o777,
			ModTime:  content.FileInfo.MTime,
			Uname:    owner,
			Gname:    group,
		})
	case files.TypeDir:
		return tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeDir,
			Name:     name + "/",
			Mode:     int64(normalizeFileMode(content.Mode())),
			ModTime:  mtime,
			Uname:    owner,
			Gname:    group,
		})
	default:
		return addTarRegular(tw, content, name, owner, group)
	}
}

func addTarRegular(tw *tar.Writer, content *files.Content, name, owner, group string) error {
	f, err := os.Open(content.Source)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     int64(normalizeFileMode(content.FileInfo.Mode)),
		Size:     fi.Size(),
		ModTime:  content.FileInfo.MTime,
		Uname:    owner,
		Gname:    group,
	}); err != nil {
		return err
	}

	_, err = io.Copy(tw, f)
	return err
}
