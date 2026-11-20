// Package dmg implements nfpm.Packager providing macOS disk images (.dmg)
// that hold an application bundle, via the pure-Go go.digitalxero.dev/osx
// library.
package dmg

import (
	"context"
	"errors"
	"io"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/internal/macapp"
	osxdmg "go.digitalxero.dev/osx/dmg"
)

const (
	packagerName = "dmg"
	extension    = ".dmg"

	// applications is the folder a user drags the application to. The image
	// carries a symbolic link to it next to the bundle.
	applications = "/Applications"
)

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// Default dmg packager.
// nolint: gochecknoglobals
var Default = &DMG{}

// DMG is a macOS disk image (.dmg) packager implementation. The image holds
// the application bundle described by the app settings and a link to the
// Applications folder.
type DMG struct{}

// ConventionalFileName returns the conventional file name for a disk image:
// name_version_arch.dmg.
func (*DMG) ConventionalFileName(info *nfpm.Info) string {
	return macapp.FileName(info, extension)
}

// ConventionalExtension returns the file extension for disk images.
func (*DMG) ConventionalExtension() string {
	return extension
}

// Package writes a new macOS disk image to the given writer using the given
// info.
func (*DMG) Package(info *nfpm.Info, w io.Writer) error {
	built, err := macapp.Build(info, packagerName)
	if err != nil {
		return err
	}

	name := macapp.BundleName(info)
	image := osxdmg.NewImage().
		WithVolumeName(name).
		WithName(name).
		WithCreated(built.ModTime)
	if err := image.AddTree(built.Bundle.FS(), "/"+built.Bundle.BundleName()); err != nil {
		return err
	}
	image.File(applications).Symlink(applications).WithModTime(built.ModTime)
	if built.Signer != nil {
		// The image is signed with the identity that signed the bundle.
		image = image.WithSigner(built.Signer)
	}

	ready, err := image.Build()
	if err != nil {
		return err
	}
	err = ready.Write(context.Background(), w)
	if errors.Is(err, osxdmg.ErrSigning) {
		return &nfpm.ErrSigningFailure{Err: err}
	}
	return err
}
