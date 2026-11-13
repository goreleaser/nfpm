// Generate focused native macOS fixtures through nFPM's public packagers:
// an installer package, an application bundle and a disk image for each of
// the non-ASCII product names the Windows fixtures use.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/app"
	"github.com/goreleaser/nfpm/v2/dmg"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/pkg"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	arch := os.Getenv("NFPM_PKG_TEST_ARCH")
	if arch == "" {
		return fmt.Errorf("set NFPM_PKG_TEST_ARCH to the architecture of dist/testapp")
	}
	data, err := os.ReadFile(filepath.Join("testdata", "acceptance", "user-cases.json"))
	if err != nil {
		return err
	}
	var cases []struct {
		Name    string `json:"name"`
		Product string `json:"product"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		return err
	}

	// The verification scripts read the cases from this list.
	var list strings.Builder
	for _, c := range cases {
		fmt.Fprintf(&list, "%s\t%s\n", c.Name, c.Product)
	}
	if err := os.WriteFile(filepath.Join("dist", "unicode-cases.tsv"), []byte(list.String()), 0o644); err != nil {
		return err
	}

	program := filepath.Join("dist", "testapp")
	for _, c := range cases {
		identifier := "dev.goreleaser.nfpm.unicode." + c.Name
		base := nfpm.Info{Name: c.Product, Version: "1.0.0", Arch: arch}

		installer := base
		installer.Contents = files.Contents{
			{
				Source:      program,
				Destination: "/usr/local/bin/nfpm-unicode-" + c.Name,
				FileInfo:    &files.ContentFileInfo{Mode: 0o755},
			},
			{
				Source:      filepath.Join("testdata", "whatever.conf"),
				Destination: "/Library/Application Support/NfpmUnicode/" + c.Product + ".conf",
				FileInfo:    &files.ContentFileInfo{Mode: 0o644},
			},
		}
		installer.Scripts.PostInstall = filepath.Join("testdata", "acceptance", "pkg", "postinstall-unicode.sh")
		installer.Pkg.Identifier = identifier

		// The bundle's main executable is named after the product, so the
		// non-ASCII name is also the file macOS has to find and launch.
		bundle := base
		bundle.Contents = files.Contents{{
			Source:      program,
			Destination: "/Contents/MacOS/" + c.Product,
			FileInfo:    &files.ContentFileInfo{Mode: 0o755},
		}}
		bundle.App.Identifier = identifier

		for _, fixture := range []struct {
			extension string
			packager  nfpm.Packager
			info      nfpm.Info
		}{
			{".pkg", pkg.Default, installer},
			{".app.zip", app.Default, bundle},
			{".dmg", dmg.Default, bundle},
		} {
			file := "unicode-" + c.Name + fixture.extension
			if err := write(filepath.Join("dist", file), fixture.packager, nfpm.WithDefaults(&fixture.info)); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
		}
	}
	return nil
}

func write(path string, packager nfpm.Packager, info *nfpm.Info) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = packager.Package(info, f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
