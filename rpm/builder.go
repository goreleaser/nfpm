package rpm

import (
	"io"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/internal/modtime"
	"go.digitalxero.dev/rpm"
)

// packageRPM builds a binary RPM and writes it to w.
func (r *RPM) packageRPM(info *nfpm.Info, w io.Writer) error {
	b := rpm.NewPackage()

	if err := applyMetadata(b, info); err != nil {
		return err
	}
	if err := applyRelations(b, info); err != nil {
		return err
	}
	if err := applyScripts(b, info); err != nil {
		return err
	}
	if err := applyChangelog(b, info); err != nil {
		return err
	}
	addContents(b, info)
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

// applyMetadata maps the nfpm.Info scalar metadata onto the package builder.
func applyMetadata(b rpm.PackageBuilder, info *nfpm.Info) error {
	b.WithName(info.Name)
	b.WithVersion(formatVersion(info))
	b.WithRelease(defaultTo(info.Release, "1"))
	b.WithSummary(defaultTo(info.RPM.Summary, strings.Split(info.Description, "\n")[0]))
	b.WithDescription(info.Description)
	b.WithArch(info.Arch)
	if info.Platform != "" {
		b.WithOS(info.Platform)
	}
	b.WithLicense(info.License)
	b.WithURL(info.Homepage)
	b.WithVendor(info.Vendor)
	b.WithGroup(info.RPM.Group)
	b.WithPackager(defaultTo(info.RPM.Packager, info.Maintainer))
	if len(info.RPM.Prefixes) > 0 {
		b.WithPrefixes(info.RPM.Prefixes...)
	}
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
