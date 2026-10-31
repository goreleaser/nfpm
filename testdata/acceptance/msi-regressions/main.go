// Generate focused native Windows fixtures through nFPM's public packager.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/msi"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	for _, fixture := range []struct {
		file, product, version, override, arch, source string
		allowSame                                      bool
	}{
		{"equal-rc", "NfpmMsiEqual", "1.2.3-rc1", "", "x64", "testapp.exe", false},
		{"equal-final-blocked", "NfpmMsiEqual", "1.2.3", "", "x64", "testapp-newer.exe", false},
		{"equal-final-allowed", "NfpmMsiEqual", "1.2.3", "", "x64", "testapp-newer.exe", true},
		{"override-rc", "NfpmMsiOverride", "2024.1.0-rc1", "24.1.0", "x64", "testapp.exe", false},
		{"override-final", "NfpmMsiOverride", "2024.1.0", "24.1.0", "x64", "testapp-newer.exe", true},
		{"arch-intel", "NfpmMsiArch", "1.0.0", "", "intel", "testapp.exe", false},
		{"arch-x86", "NfpmMsiArch", "2.0.0", "", "x86", "testapp-newer.exe", false},
	} {
		info := nfpm.WithDefaults(&nfpm.Info{
			Name: fixture.product, Version: fixture.version, Arch: "amd64", Vendor: "nFPM",
			Overridables: nfpm.Overridables{
				Contents: files.Contents{{Source: filepath.Join("dist", fixture.source), Destination: "/app.exe"}},
				MSI: nfpm.MSI{
					Arch: fixture.arch, Version: fixture.override,
					FileVersions: map[string]string{"/app.exe": ""},
					Upgrade:      nfpm.MSIUpgrade{AllowSameVersion: fixture.allowSame},
				},
			},
		})
		f, err := os.Create(filepath.Join("dist", fixture.file+".msi"))
		if err != nil {
			return err
		}
		err = msi.Default.Package(info, f)
		closeErr := f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", fixture.file, err)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
