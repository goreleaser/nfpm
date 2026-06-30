// Package rpm implements nfpm.Packager providing .rpm and .src.rpm bindings
// using go.digitalxero.dev/rpm.
package rpm

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/internal/sign"
)

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(formatRPM.String(), DefaultRPM)
	nfpm.RegisterPackager(formatSRPM.String(), DefaultSRPM)
}

// DefaultRPM RPM packager.
// nolint: gochecknoglobals
var DefaultRPM = &RPM{formatRPM}

// DefaultSRPM SRPM packager.
// nolint: gochecknoglobals
var DefaultSRPM = &RPM{formatSRPM}

type format uint

const (
	formatRPM format = iota
	formatSRPM
)

// String implements fmt.Stringer.
func (f format) String() string { return [2]string{"rpm", "srpm"}[f] }

// RPM is a RPM packager implementation.
type RPM struct {
	format format
}

// https://docs.fedoraproject.org/ro/Fedora_Draft_Documentation/0.1/html/RPM_Guide/ch01s03.html
// https://github.com/rpm-software-management/rpm/blob/4a9b7b5908d8b463a836b51322242677677bd8b7/lib/rpmrc.cc#L1167
// nolint: gochecknoglobals
var archToRPM = map[string]string{
	"all":      "noarch",
	"amd64":    "x86_64",
	"386":      "i386",
	"arm64":    "aarch64",
	"arm5":     "armv5tel",
	"arm6":     "armv6hl",
	"arm7":     "armv7hl",
	"mips64le": "mips64el",
	"mipsle":   "mipsel",
	"mips":     "mips",
	"loong64":  "loongarch64",
	"riscv64":  "riscv64",
}

func setDefaults(info *nfpm.Info) *nfpm.Info {
	if info.RPM.Arch != "" {
		info.Arch = info.RPM.Arch
	} else if arch, ok := archToRPM[info.Arch]; ok {
		info.Arch = arch
	}

	info.Release = cmp.Or(info.Release, "1")
	info.RPM.Compression = cmp.Or(info.RPM.Compression, "gzip")
	return info
}

// ConventionalFileName returns a file name according
// to the conventions for RPM packages. See:
// http://ftp.rpm.org/max-rpm/ch-rpm-file-format.html
func (r *RPM) ConventionalFileName(info *nfpm.Info) string {
	info = setDefaults(info)

	// Source packages carry their arch (src) in the extension, so they are named
	// name-version-release.src.rpm without a separate architecture component.
	if r.format == formatSRPM {
		return fmt.Sprintf(
			"%s-%s-%s%s",
			info.Name,
			formatVersion(info),
			defaultTo(info.Release, "1"),
			r.ConventionalExtension(),
		)
	}

	// name-version-release.architecture.rpm
	return fmt.Sprintf(
		"%s-%s-%s.%s%s",
		info.Name,
		formatVersion(info),
		defaultTo(info.Release, "1"),
		info.Arch,
		r.ConventionalExtension(),
	)
}

// ConventionalExtension returns the file name conventionally used for RPM packages
func (r *RPM) ConventionalExtension() string {
	if r.format == formatSRPM {
		return ".src.rpm"
	}
	return ".rpm"
}

// contentPackager is the packager name used to select and prepare contents.
// Both .rpm and .src.rpm use "rpm": a source package bundles the very contents
// that build the binary rpm, and RPM-specific content types (doc/ghost/license/
// readme) are only retained by files.PrepareForPackager for the "rpm" packager.
const contentPackager = "rpm"

// Package writes a new RPM package to the given writer using the given info.
func (r *RPM) Package(info *nfpm.Info, w io.Writer) (err error) {
	info = setDefaults(info)

	if err = nfpm.PrepareForPackager(info, contentPackager); err != nil {
		return err
	}

	if r.format == formatSRPM {
		return r.packageSRPM(info, w)
	}
	return r.packageRPM(info, w)
}

func formatVersion(info *nfpm.Info) string {
	version := info.Version

	if info.Prerelease != "" {
		version += "~" + strings.ReplaceAll(info.Prerelease, "-", "_")
	}

	if info.VersionMetadata != "" {
		version += "+" + info.VersionMetadata
	}

	return version
}

func defaultTo(in, def string) string {
	if in == "" {
		return def
	}
	return in
}

// parseEpoch parses the configured epoch. The boolean result reports whether an
// epoch was configured at all; an unset epoch must not be written to the header.
func parseEpoch(info *nfpm.Info) (uint32, bool, error) {
	if info.Epoch == "" {
		return 0, false, nil
	}
	epoch, err := strconv.ParseUint(info.Epoch, 10, 32)
	if err != nil {
		return 0, false, err
	}
	return uint32(epoch), true, nil
}

// buildHost returns the configured build host, defaulting to the OS hostname.
func buildHost(info *nfpm.Info) (string, error) {
	if info.RPM.BuildHost != "" {
		return info.RPM.BuildHost, nil
	}
	return os.Hostname()
}

// signFunc returns the PGP signing function for the package, or nil when signing
// is not configured. A custom SignFn takes precedence over a key file, matching
// the previous behavior where it was registered last.
func signFunc(info *nfpm.Info) func([]byte) ([]byte, error) {
	if signFn := info.RPM.Signature.SignFn; signFn != nil {
		return func(data []byte) ([]byte, error) {
			return signFn(bytes.NewReader(data))
		}
	}
	if info.RPM.Signature.KeyFile != "" {
		return sign.PGPSignerWithKeyID(
			info.RPM.Signature.KeyFile,
			info.RPM.Signature.KeyPassphrase,
			info.RPM.Signature.KeyID,
		)
	}
	return nil
}
