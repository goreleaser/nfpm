package msix

import (
	"bytes"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "MyCompany.TestApp",
		Arch:        "amd64",
		Description: "Test application",
		Version:     "v1.0.0",
		Maintainer:  "Test <test@example.com>",
		Vendor:      "TestCo",
		Homepage:    "https://example.com",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/app/fake.exe",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/app/config.conf",
				},
			},
			MSIX: nfpm.MSIX{
				Publisher: "CN=TestCompany, O=TestCompany, C=US",
				Properties: nfpm.MSIXProperties{
					Logo: "app/fake.exe",
				},
				Applications: []nfpm.MSIXApplication{
					{
						ID:         "App",
						Executable: "app/fake.exe",
						EntryPoint: "Windows.FullTrustApplication",
					},
				},
			},
		},
	})
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".msix", Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo()
	name := Default.ConventionalFileName(info)
	require.Equal(t, "MyCompany.TestApp_1.0.0.0_x64.msix", name)
}

func TestArchMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"amd64", "x64"},
		{"x86_64", "x64"},
		{"386", "x86"},
		{"i386", "x86"},
		{"i686", "x86"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
		{"arm", "arm"},
		{"arm7", "arm"},
		{"all", "neutral"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = tt.input
			info.MSIX.Arch = ""
			info = ensureValidArch(info)
			require.Equal(t, tt.expected, info.Arch)
		})
	}
}

func TestArchOverride(t *testing.T) {
	info := exampleInfo()
	info.Arch = "amd64"
	info.MSIX.Arch = "x86"
	info = ensureValidArch(info)
	require.Equal(t, "x86", info.Arch)
}

func TestVersionConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.2.3", "1.2.3.0"},
		{"v1.2.3", "1.2.3.0"},
		{"1.0.0", "1.0.0.0"},
		{"2.5", "2.5.0.0"},
		{"1", "1.0.0.0"},
		{"1.2.3.4", "1.2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertToMSIXVersion(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPackageMinimal(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	require.Positive(t, buf.Len(), "package should not be empty")
}

func TestPackageWithContents(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/whatever.conf",
		Destination: "/app/extra.txt",
	})
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	require.Positive(t, buf.Len())
}

func TestNoInfo(t *testing.T) {
	info := nfpm.WithDefaults(&nfpm.Info{})
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.Error(t, err)
}

func TestMissingPublisher(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Publisher = ""
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msix.publisher")
}

func TestMissingLogo(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Properties.Logo = ""
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msix.properties.logo")
}

func TestMissingApplications(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Applications = nil
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msix.applications")
}

func TestMissingApplicationID(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Applications[0].ID = ""
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msix.applications[0].id")
}

func TestMissingApplicationExecutable(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Applications[0].Executable = ""
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msix.applications[0].executable")
}

func TestSetPackagerDefaults(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Applications[0].EntryPoint = ""
	info.MSIX.Applications[0].VisualElements.BackgroundColor = ""
	info.MSIX.Applications[0].VisualElements.Square150x150Logo = ""
	info.MSIX.Applications[0].VisualElements.Square44x44Logo = ""
	info.MSIX.Dependencies.TargetDeviceFamilies = nil
	info.MSIX.Capabilities.Restricted = nil

	Default.SetPackagerDefaults(info)

	require.Equal(t, "Windows.FullTrustApplication", info.MSIX.Applications[0].EntryPoint)
	require.Equal(t, "transparent", info.MSIX.Applications[0].VisualElements.BackgroundColor)
	require.Equal(t, info.MSIX.Properties.Logo, info.MSIX.Applications[0].VisualElements.Square150x150Logo)
	require.Equal(t, info.MSIX.Properties.Logo, info.MSIX.Applications[0].VisualElements.Square44x44Logo)
	require.Len(t, info.MSIX.Dependencies.TargetDeviceFamilies, 1)
	require.Equal(t, "Windows.Desktop", info.MSIX.Dependencies.TargetDeviceFamilies[0].Name)
	require.Contains(t, info.MSIX.Capabilities.Restricted, "runFullTrust")
}

func TestPackageWithCustomProperties(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Properties = nfpm.MSIXProperties{
		DisplayName:          "My Custom App",
		PublisherDisplayName: "My Company",
		Logo:                 "Assets/logo.png",
	}
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	require.Positive(t, buf.Len())
}

func TestPackageWithCapabilities(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Capabilities = nfpm.MSIXCapabilities{
		Capabilities:       []string{"internetClient"},
		DeviceCapabilities: []string{"microphone"},
		Restricted:         []string{"broadFileSystemAccess"},
	}
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	require.Positive(t, buf.Len())
}

func TestPackageWithDependencies(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Dependencies = nfpm.MSIXDependencies{
		TargetDeviceFamilies: []nfpm.MSIXTargetDeviceFamily{
			{
				Name:             "Windows.Desktop",
				MinVersion:       "10.0.19041.0",
				MaxVersionTested: "10.0.22621.0",
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	require.Positive(t, buf.Len())
}
