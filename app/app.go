// Package app implements nfpm.Packager providing macOS application bundles,
// delivered as a zip archive (.app.zip), via the pure-Go
// go.digitalxero.dev/osx library.
package app

import (
	"context"
	"io"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/internal/macapp"
)

const (
	packagerName = "app"
	extension    = ".app.zip"
)

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// Default app packager.
// nolint: gochecknoglobals
var Default = &App{}

// App is a macOS application bundle (.app.zip) packager implementation.
type App struct{}

// ConventionalFileName returns the conventional file name for a zipped
// application bundle: name_version_arch.app.zip.
func (*App) ConventionalFileName(info *nfpm.Info) string {
	return macapp.FileName(info, extension)
}

// ConventionalExtension returns the file extension for zipped application
// bundles.
func (*App) ConventionalExtension() string {
	return extension
}

// Package writes a new zipped macOS application bundle to the given writer
// using the given info.
func (*App) Package(info *nfpm.Info, w io.Writer) error {
	built, err := macapp.Build(info, packagerName)
	if err != nil {
		return err
	}
	return built.Bundle.Write(context.Background(), w)
}
