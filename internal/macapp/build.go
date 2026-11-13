package macapp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/macos"
	"github.com/goreleaser/nfpm/v2/internal/modtime"
	osxapp "go.digitalxero.dev/osx/app"
	"go.digitalxero.dev/osx/sign"
)

// Built is a bundle ready to be written, with what its container needs to
// know about it.
type Built struct {
	// Bundle is the built, and when configured signed, application bundle.
	Bundle osxapp.Bundle
	// Signer is the identity the bundle was signed with, nil when unsigned.
	// The dmg packager signs its image with the same identity.
	Signer sign.CodeSigner
	// ModTime is the time stamped on everything nFPM generates.
	ModTime time.Time
}

// Arch returns the architecture the bundle is built for, in macOS
// nomenclature.
func Arch(info *nfpm.Info) (string, error) {
	return macos.ResolveArch("app.arch", info.App.Arch, info.Arch)
}

// FileName returns the conventional file name of a bundle delivered with the
// given extension: name_version_arch.extension. The version carries the
// prerelease, metadata and release, so two distinct releases never share a
// file name.
func FileName(info *nfpm.Info, extension string) string {
	arch, err := Arch(info)
	if err != nil {
		// Build reports the unsupported architecture; keep the name
		// informative until then.
		arch = info.Arch
	}
	return fmt.Sprintf("%s_%s_%s%s", info.Name, Version(info), arch, extension)
}

// Build prepares the contents for the named packager and builds the bundle
// they describe. The caller's info is not changed.
func Build(info *nfpm.Info, packager string) (*Built, error) {
	// Preparation replaces the contents; work on a copy so the caller's Info
	// can be packaged again.
	prepared := *info
	info = &prepared

	arch, err := Arch(info)
	if err != nil {
		return nil, err
	}
	if err := validateSettings(info); err != nil {
		return nil, err
	}
	if err := nfpm.PrepareForPackager(info, packager); err != nil {
		return nil, err
	}
	warnUnsupported(packager, info)

	entries, err := bundleEntries(packager, info, arch)
	if err != nil {
		return nil, err
	}

	mtime := modtime.Get(info.MTime)
	builder := osxapp.NewBundle().
		WithName(BundleName(info)).
		WithBundleID(info.App.Identifier).
		WithVersion(Version(info)).
		WithBuildVersion(info.Version).
		WithMainExecutable(executableName(info)).
		WithModTime(mtime)
	if info.App.MinOSVersion != "" {
		builder = builder.WithMinimumSystemVersion(info.App.MinOSVersion)
	}
	// The keys are added in a fixed order. The property list encoder sorts
	// them again, so this is not what makes the output reproducible, but
	// nothing here should depend on how the library stores them.
	for _, key := range slices.Sorted(maps.Keys(info.App.InfoPlist)) {
		builder = builder.WithInfoPlistValue(key, info.App.InfoPlist[key])
	}
	for _, entry := range entries {
		file := builder.File("Contents/" + entry.path).WithModTime(entry.content.ModTime())
		switch entry.content.Type {
		case files.TypeDir:
			file.Directory().WithMode(entry.content.Mode())
		case files.TypeSymlink:
			file.Symlink(entry.content.Source)
		default:
			if _, err := os.Stat(entry.content.Source); err != nil {
				return nil, fmt.Errorf("reading file %s: %w", entry.content.Source, err)
			}
			file.FromPath(entry.content.Source).WithMode(entry.content.Mode())
		}
	}

	built := &Built{ModTime: mtime}
	if info.App.Signature.P12File != "" {
		built.Signer, err = newSigner(packager, info)
		if err != nil {
			return nil, err
		}
		builder = builder.WithCodeSigner(built.Signer)
	} else if info.App.Signature.Entitlements != "" {
		return nil, errors.New("package app.signature.entitlements needs app.signature.p12_file: entitlements are part of a signature")
	}

	built.Bundle, err = builder.BuildContext(context.Background())
	if err != nil {
		if errors.Is(err, osxapp.ErrSigning) {
			return nil, &nfpm.ErrSigningFailure{Err: err}
		}
		return nil, err
	}
	return built, nil
}

// warnUnsupported reports the shared settings an application bundle has no
// place for.
func warnUnsupported(packager string, info *nfpm.Info) {
	macos.WarnRelationships(packager, info)
	macos.WarnScripts(packager, "an application bundle is not installed by a package manager",
		[]string{"preinstall", "postinstall", "preremove", "postremove"},
		[]string{info.Scripts.PreInstall, info.Scripts.PostInstall, info.Scripts.PreRemove, info.Scripts.PostRemove})
}

// newSigner builds the code signer for the configured PKCS#12 identity.
func newSigner(packager string, info *nfpm.Info) (sign.CodeSigner, error) {
	signature := info.App.Signature
	identity, err := macos.ReadP12(signature.P12File)
	if err != nil {
		return nil, err
	}
	builder := sign.NewCodeSigner().WithP12(identity, signature.KeyPassphrase)
	if url, override := macos.TimestampURL(signature.TimestampURL); override {
		builder = builder.WithTimestampURL(url)
	}
	if signature.Entitlements != "" {
		entitlements, err := os.ReadFile(signature.Entitlements)
		if err != nil {
			return nil, fmt.Errorf("reading entitlements %s: %w", signature.Entitlements, err)
		}
		builder = builder.WithEntitlements(entitlements)
	}
	signer, err := builder.Build()
	if err != nil {
		return nil, &nfpm.ErrSigningFailure{Err: fmt.Errorf("building signer: %w", err)}
	}
	macos.WarnCertificateKind(packager, signature.P12File, signer.Certificates(), macos.DeveloperIDApplication)
	return signer, nil
}
