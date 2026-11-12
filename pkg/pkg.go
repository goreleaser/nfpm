// Package pkg implements nfpm.Packager providing macOS installer packages
// (.pkg) via the pure-Go go.digitalxero.dev/osx library.
package pkg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/macos"
	"github.com/goreleaser/nfpm/v2/internal/modtime"
	"go.digitalxero.dev/osx/cms"
	osxpkg "go.digitalxero.dev/osx/pkg"
)

const packagerName = "pkg"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// Default pkg packager.
// nolint: gochecknoglobals
var Default = &Pkg{}

// Pkg is a macOS installer package (.pkg) packager implementation.
type Pkg struct{}

// nolint: gochecknoglobals
var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	osVersionPattern  = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(\.[0-9]+)?$`)
)

// ConventionalFileName returns the conventional file name for a macOS
// installer package: name_version_arch.pkg. The version carries the
// prerelease, metadata and release, so two distinct releases never share a
// file name.
func (*Pkg) ConventionalFileName(info *nfpm.Info) string {
	arch, err := macos.ResolveArch("pkg.arch", info.Pkg.Arch, info.Arch)
	if err != nil {
		// Package reports the unsupported architecture; keep the name
		// informative until then.
		arch = info.Arch
	}
	return fmt.Sprintf("%s_%s_%s.pkg", info.Name, packageVersion(info), arch)
}

// ConventionalExtension returns the file extension for macOS installer
// packages.
func (*Pkg) ConventionalExtension() string {
	return ".pkg"
}

// packageVersion returns the version macOS records for the installation. It
// keeps every part of the release identity.
func packageVersion(info *nfpm.Info) string {
	version := info.Version
	if info.Prerelease != "" {
		version += "-" + info.Prerelease
	}
	if info.VersionMetadata != "" {
		version += "+" + info.VersionMetadata
	}
	if info.Release != "" {
		version += "-" + info.Release
	}
	return version
}

// Package writes a new macOS installer package to the given writer using the
// given info.
func (*Pkg) Package(info *nfpm.Info, w io.Writer) error {
	// Preparation replaces the contents; work on a copy so the caller's Info
	// can be packaged again.
	prepared := *info
	info = &prepared

	arch, err := macos.ResolveArch("pkg.arch", info.Pkg.Arch, info.Arch)
	if err != nil {
		return err
	}
	if err := validateSettings(info); err != nil {
		return err
	}
	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}
	warnUnsupported(info)

	version := packageVersion(info)
	// The package time is SOURCE_DATE_EPOCH or the configured mtime when set,
	// which is what makes a build reproducible.
	mtime := modtime.Get(info.MTime)
	component := osxpkg.NewPackage().
		WithName(info.Name).
		WithIdentifier(info.Pkg.Identifier).
		WithVersion(version).
		WithInstallLocation("/").
		WithModTime(mtime)
	if err := addContents(component, info); err != nil {
		return err
	}
	if err := addScripts(component, info); err != nil {
		return err
	}

	product := osxpkg.NewProduct().
		WithTitle(info.Name).
		WithIdentifier(info.Pkg.Identifier).
		WithVersion(version).
		WithHostArchitectures(macos.HostArchitectures(arch)...).
		WithModTime(mtime).
		WithComponent(component)
	if info.Pkg.MinOSVersion != "" {
		product = product.WithMinOSVersion(info.Pkg.MinOSVersion)
	}
	if info.Pkg.Signature.P12File != "" {
		signer, err := newSigner(info)
		if err != nil {
			return err
		}
		product = product.WithSigner(signer)
	}

	built, err := product.Build()
	if err != nil {
		return describeBuildError(err)
	}
	err = built.Write(context.Background(), w)
	if errors.Is(err, osxpkg.ErrSigning) {
		return &nfpm.ErrSigningFailure{Err: err}
	}
	return err
}

// describeBuildError names the contents field behind an owner or group the
// library could not resolve; other errors are already specific.
func describeBuildError(err error) error {
	var unknown *osxpkg.UnknownAccountError
	if errors.As(err, &unknown) {
		field := "owner"
		if unknown.Kind == "group" {
			field = "group"
		}
		return fmt.Errorf(
			"package contents %q: %s %q is not a built-in macOS account (owners: %s; groups: %s), use its numeric ID: %w",
			unknown.Path, field, unknown.Name,
			strings.Join(osxpkg.BuiltinUsers(), ", "), strings.Join(osxpkg.BuiltinGroups(), ", "), err)
	}
	return err
}

// validateSettings checks the pkg-specific settings that do not depend on the
// contents.
func validateSettings(info *nfpm.Info) error {
	if info.Pkg.Identifier == "" {
		return errors.New("package pkg.identifier must be provided, for example com.example." + info.Name)
	}
	if !identifierPattern.MatchString(info.Pkg.Identifier) {
		return fmt.Errorf(
			"package pkg.identifier %q is not valid: use letters, digits, periods, hyphens and underscores, for example com.example.myapp",
			info.Pkg.Identifier)
	}
	if v := info.Pkg.MinOSVersion; v != "" && !osVersionPattern.MatchString(v) {
		return fmt.Errorf("package pkg.min_os_version %q is not valid: use a macOS version such as 13.0", v)
	}
	return nil
}

// warnUnsupported reports the shared settings a macOS installer package has
// no place for.
func warnUnsupported(info *nfpm.Info) {
	macos.WarnRelationships(packagerName, info)
	macos.WarnScripts(packagerName, "macOS has no package removal",
		[]string{"preremove", "postremove"},
		[]string{info.Scripts.PreRemove, info.Scripts.PostRemove})
}

// addContents adds every prepared contents entry to the component package.
func addContents(component osxpkg.PackageBuilder, info *nfpm.Info) error {
	index := macos.NewPathIndex()
	// installed maps every path an entry is installed at to the destination
	// that put it there: /etc/foo and /private/etc/foo are one file.
	installed := map[string]string{}
	added := 0
	for _, content := range info.Contents {
		if content.Type == files.TypeImplicitDir {
			// Parent directories are created by the installer when missing.
			// Listing them would make the package claim directories such as
			// /usr/local that it does not own.
			continue
		}

		destination := canonicalDestination(content.Destination)
		if content.Type == files.TypeDir && isSystemDirectory(destination) {
			// A tree rooted at / lists directories such as /Library. They
			// are not the package's to create or change.
			continue
		}
		if err := validateDestination(content.Destination, destination); err != nil {
			return err
		}
		if previous, taken := installed[destination]; taken {
			return fmt.Errorf(
				"package contents %q and %q are installed at the same path, %s",
				previous, content.Destination, destination)
		}
		installed[destination] = content.Destination
		if err := index.Add(destination); err != nil {
			return err
		}
		// The library resolves the built-in macOS accounts and numeric IDs, and
		// reports any other name when the package is built.
		entry := component.File(destination).
			WithOwner(content.FileInfo.Owner).
			WithGroup(content.FileInfo.Group).
			WithModTime(content.ModTime())
		switch content.Type {
		case files.TypeDir:
			entry.Directory().WithMode(content.Mode())
		case files.TypeSymlink:
			entry.Symlink(content.Source)
		case files.TypeConfig, files.TypeConfigNoReplace, files.TypeConfigMissingOK:
			log.Printf(
				"warning: pkg: macOS has no configuration file handling, %s is installed as a regular file and replaced on upgrade",
				content.Destination)
			fallthrough
		default:
			if _, err := os.Stat(content.Source); err != nil {
				return fmt.Errorf("reading file %s: %w", content.Source, err)
			}
			entry.FromPath(content.Source).WithMode(content.Mode())
		}
		added++
	}
	if added == 0 {
		return errors.New("package contents must include at least one file, directory or symlink")
	}
	return nil
}

// addScripts attaches the install scripts macOS supports.
func addScripts(component osxpkg.PackageBuilder, info *nfpm.Info) error {
	for _, script := range []struct {
		name string
		path string
		add  func([]byte) osxpkg.PackageBuilder
	}{
		{"preinstall", info.Scripts.PreInstall, component.WithPreInstallScript},
		{"postinstall", info.Scripts.PostInstall, component.WithPostInstallScript},
	} {
		if script.path == "" {
			continue
		}
		body, err := os.ReadFile(script.path)
		if err != nil {
			return fmt.Errorf("reading %s script %s: %w", script.name, script.path, err)
		}
		if !strings.HasPrefix(string(body), "#!") {
			return fmt.Errorf(
				"package scripts.%s %s must start with an interpreter line such as #!/bin/sh; the macOS installer runs it directly",
				script.name, script.path)
		}
		script.add(body)
	}
	return nil
}

// newSigner builds the signer for the configured PKCS#12 identity.
func newSigner(info *nfpm.Info) (osxpkg.Signer, error) {
	path := info.Pkg.Signature.P12File
	data, err := macos.ReadP12(path)
	if err != nil {
		return nil, err
	}

	builder := cms.NewSigner().WithP12(data, info.Pkg.Signature.KeyPassphrase)
	if url, override := macos.TimestampURL(info.Pkg.Signature.TimestampURL); override {
		builder = builder.WithTimestampURL(url)
	}
	signer, err := builder.Build()
	if err != nil {
		return nil, &nfpm.ErrSigningFailure{Err: fmt.Errorf("building signer: %w", err)}
	}
	macos.WarnCertificateKind(packagerName, path, signer.Certificates(), macos.DeveloperIDInstaller)
	return signer, nil
}
