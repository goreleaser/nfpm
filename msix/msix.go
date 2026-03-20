// Package msix implements nfpm.Packager providing .msix bindings.
package msix

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"go.digitalxero.dev/go-msix"
)

const packagerName = "msix"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// nolint: gochecknoglobals
var archToMSIX = map[string]string{
	"amd64":   "x64",
	"x86_64":  "x64",
	"386":     "x86",
	"i386":    "x86",
	"i686":    "x86",
	"arm64":   "arm64",
	"aarch64": "arm64",
	"arm":     "arm",
	"arm7":    "arm",
	"all":     "neutral",
}

func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	if info.MSIX.Arch != "" {
		info.Arch = info.MSIX.Arch
	} else if arch, ok := archToMSIX[info.Arch]; ok {
		info.Arch = arch
	}
	return info
}

// Default msix packager.
// nolint: gochecknoglobals
var Default = &MSIX{}

// MSIX is an msix packager implementation.
type MSIX struct{}

// ConventionalFileName returns the conventional file name for an MSIX package.
func (m *MSIX) ConventionalFileName(info *nfpm.Info) string {
	info = ensureValidArch(info)
	version := convertToMSIXVersion(info.Version)
	return fmt.Sprintf("%s_%s_%s.msix", info.Name, version, info.Arch)
}

// ConventionalExtension returns the file extension for MSIX packages.
func (*MSIX) ConventionalExtension() string {
	return ".msix"
}

// SetPackagerDefaults sets default values for MSIX-specific fields.
func (*MSIX) SetPackagerDefaults(info *nfpm.Info) {
	needsFullTrust := false
	for i := range info.MSIX.Applications {
		if info.MSIX.Applications[i].EntryPoint == "" {
			info.MSIX.Applications[i].EntryPoint = "Windows.FullTrustApplication"
		}
		if info.MSIX.Applications[i].VisualElements.BackgroundColor == "" {
			info.MSIX.Applications[i].VisualElements.BackgroundColor = "transparent"
		}
		// Default Square150x150Logo and Square44x44Logo to the package Logo
		if info.MSIX.Applications[i].VisualElements.Square150x150Logo == "" && info.MSIX.Properties.Logo != "" {
			info.MSIX.Applications[i].VisualElements.Square150x150Logo = info.MSIX.Properties.Logo
		}
		if info.MSIX.Applications[i].VisualElements.Square44x44Logo == "" && info.MSIX.Properties.Logo != "" {
			info.MSIX.Applications[i].VisualElements.Square44x44Logo = info.MSIX.Properties.Logo
		}
		if info.MSIX.Applications[i].EntryPoint == "Windows.FullTrustApplication" {
			needsFullTrust = true
		}
	}

	// Auto-add runFullTrust restricted capability when any app uses FullTrustApplication
	if needsFullTrust {
		hasRunFullTrust := false
		for _, c := range info.MSIX.Capabilities.Restricted {
			if c == "runFullTrust" {
				hasRunFullTrust = true
				break
			}
		}
		if !hasRunFullTrust {
			info.MSIX.Capabilities.Restricted = append(info.MSIX.Capabilities.Restricted, "runFullTrust")
		}
	}

	if len(info.MSIX.Dependencies.TargetDeviceFamilies) == 0 {
		info.MSIX.Dependencies.TargetDeviceFamilies = []nfpm.MSIXTargetDeviceFamily{
			{
				Name:             "Windows.Desktop",
				MinVersion:       "10.0.17763.0",
				MaxVersionTested: "10.0.22621.0",
			},
		}
	}
}

// Package writes a new MSIX package to the given writer using the given info.
func (m *MSIX) Package(info *nfpm.Info, w io.Writer) error {
	m.SetPackagerDefaults(info)
	info = ensureValidArch(info)

	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}

	if err := validate(info); err != nil {
		return err
	}

	builder := msix.NewBuilder()

	builder.Manifest = msix.Manifest{
		Identity: msix.Identity{
			Name:                  info.Name,
			Version:               convertToMSIXVersion(info.Version),
			Publisher:             info.MSIX.Publisher,
			ProcessorArchitecture: info.Arch,
			ResourceID:            info.MSIX.Identity.ResourceID,
		},
		Properties: buildProperties(info),
		Dependencies: msix.Dependencies{
			TargetDeviceFamilies: buildTargetDeviceFamilies(info),
		},
		Resources: []msix.Resource{
			{Language: "en-us"},
		},
		Applications: buildApplications(info),
		Capabilities: buildCapabilities(info),
	}

	if err := addContents(builder, info); err != nil {
		return err
	}

	if info.MSIX.Signature.PFXFile != "" {
		if err := configureSigning(builder, info); err != nil {
			return err
		}
	}

	return builder.Build(w)
}

func validate(info *nfpm.Info) error {
	if info.MSIX.Publisher == "" {
		return fmt.Errorf("package %s must be provided", "msix.publisher")
	}
	if info.MSIX.Properties.Logo == "" {
		return fmt.Errorf("package %s must be provided", "msix.properties.logo")
	}
	if len(info.MSIX.Applications) == 0 {
		return fmt.Errorf("package %s must be provided", "msix.applications")
	}
	for i, app := range info.MSIX.Applications {
		if app.ID == "" {
			return fmt.Errorf("package %s must be provided", fmt.Sprintf("msix.applications[%d].id", i))
		}
		if app.Executable == "" {
			return fmt.Errorf("package %s must be provided", fmt.Sprintf("msix.applications[%d].executable", i))
		}
	}
	return nil
}

func buildProperties(info *nfpm.Info) msix.Properties {
	props := msix.Properties{
		DisplayName:          info.MSIX.Properties.DisplayName,
		PublisherDisplayName: info.MSIX.Properties.PublisherDisplayName,
		Logo:                 info.MSIX.Properties.Logo,
		Description:          info.Description,
	}
	if props.DisplayName == "" {
		props.DisplayName = info.Name
	}
	if props.PublisherDisplayName == "" {
		props.PublisherDisplayName = info.Name
	}
	return props
}

func buildTargetDeviceFamilies(info *nfpm.Info) []msix.TargetDeviceFamily {
	families := make([]msix.TargetDeviceFamily, len(info.MSIX.Dependencies.TargetDeviceFamilies))
	for i, f := range info.MSIX.Dependencies.TargetDeviceFamilies {
		families[i] = msix.TargetDeviceFamily{
			Name:             f.Name,
			MinVersion:       f.MinVersion,
			MaxVersionTested: f.MaxVersionTested,
		}
	}
	return families
}

func buildApplications(info *nfpm.Info) []msix.Application {
	apps := make([]msix.Application, len(info.MSIX.Applications))
	for i, app := range info.MSIX.Applications {
		apps[i] = msix.Application{
			ID:         app.ID,
			Executable: app.Executable,
			EntryPoint: app.EntryPoint,
			VisualElements: msix.VisualElements{
				DisplayName:       app.VisualElements.DisplayName,
				Description:       app.VisualElements.Description,
				BackgroundColor:   app.VisualElements.BackgroundColor,
				Square150x150Logo: app.VisualElements.Square150x150Logo,
				Square44x44Logo:   app.VisualElements.Square44x44Logo,
			},
		}
		if apps[i].VisualElements.DisplayName == "" {
			apps[i].VisualElements.DisplayName = info.Name
		}
		if apps[i].VisualElements.Description == "" {
			apps[i].VisualElements.Description = info.Description
		}
	}
	return apps
}

func buildCapabilities(info *nfpm.Info) msix.Capabilities {
	caps := msix.Capabilities{}
	for _, c := range info.MSIX.Capabilities.Capabilities {
		caps.Capabilities = append(caps.Capabilities, msix.Capability{Name: c})
	}
	for _, c := range info.MSIX.Capabilities.DeviceCapabilities {
		caps.DeviceCapabilities = append(caps.DeviceCapabilities, msix.DeviceCapability{Name: c})
	}
	for _, c := range info.MSIX.Capabilities.Restricted {
		caps.Restricted = append(caps.Restricted, msix.RestrictedCapability{Name: c})
	}
	return caps
}

func addContents(builder *msix.Builder, info *nfpm.Info) error {
	for _, content := range info.Contents {
		switch content.Type {
		case files.TypeDir:
			// Directories are implicit in MSIX — skip
			continue
		case files.TypeSymlink:
			log.Printf("warning: msix does not support symlinks, skipping %s", content.Destination)
			continue
		default:
			// Treat everything else (TypeFile, TypeConfig, etc.) as regular files
			dest := normalizePathForMSIX(content.Destination)
			if content.Source != "" {
				if err := builder.AddFile(dest, content.Source); err != nil {
					return fmt.Errorf("adding file %s: %w", content.Source, err)
				}
			}
		}
	}
	return nil
}

func configureSigning(builder *msix.Builder, info *nfpm.Info) error {
	pfxPath := info.MSIX.Signature.PFXFile
	if _, err := os.Stat(pfxPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("PFX file not found: %w", err)
		}
		return fmt.Errorf("unable to access PFX file: %w", err)
	}

	cert, key, chain, err := msix.LoadPFX(pfxPath, info.MSIX.Signature.KeyPassphrase)
	if err != nil {
		return &nfpm.ErrSigningFailure{Err: fmt.Errorf("loading PFX: %w", err)}
	}

	builder.SignOptions = &msix.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		CertChain:   chain,
	}

	return nil
}

// normalizePathForMSIX converts Unix-style paths to Windows-style package paths.
func normalizePathForMSIX(path string) string {
	// Remove leading slash
	path = strings.TrimPrefix(path, "/")
	// Convert forward slashes — go-msix normalizes internally but be explicit
	return path
}

// convertToMSIXVersion converts a semver-style version to MSIX's 4-part format.
// MSIX requires Major.Minor.Build.Revision format.
func convertToMSIXVersion(version string) string {
	version = strings.TrimPrefix(version, "v")

	// Split on dots
	parts := strings.SplitN(version, ".", 4)

	// Ensure all parts are valid numbers, default to 0
	result := make([]string, 4)
	for i := range 4 {
		if i < len(parts) {
			if _, err := strconv.Atoi(parts[i]); err == nil {
				result[i] = parts[i]
			} else {
				result[i] = "0"
			}
		} else {
			result[i] = "0"
		}
	}

	return strings.Join(result, ".")
}
