// Package msix implements nfpm.Packager providing .msix bindings.
package msix

import (
	"context"
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

	builder := msix.NewBuilder().
		WithIdentity(msix.NewIdentity().
			WithName(info.Name).
			WithVersion(convertToMSIXVersion(info.Version)).
			WithPublisher(info.MSIX.Publisher).
			WithProcessorArchitecture(info.Arch).
			WithResourceID(info.MSIX.Identity.ResourceID).
			Build()).
		WithProperties(buildProperties(info)).
		WithDependencies(buildDependencies(info)).
		WithCapabilities(buildCapabilities(info)).
		AddResource(msix.NewResource().WithLanguage("en-us").Build())

	for _, app := range buildApplications(info) {
		builder.AddApplication(app)
	}

	addContents(builder, info)

	if info.MSIX.Signature.PFXFile != "" {
		if err := configureSigning(builder, info); err != nil {
			return err
		}
	}

	return builder.Build(context.Background(), w)
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
	displayName := info.MSIX.Properties.DisplayName
	if displayName == "" {
		displayName = info.Name
	}
	publisherDisplayName := info.MSIX.Properties.PublisherDisplayName
	if publisherDisplayName == "" {
		publisherDisplayName = info.Name
	}
	return msix.NewProperties().
		WithDisplayName(displayName).
		WithPublisherDisplayName(publisherDisplayName).
		WithLogo(info.MSIX.Properties.Logo).
		WithDescription(info.Description).
		Build()
}

func buildDependencies(info *nfpm.Info) msix.Dependencies {
	deps := msix.NewDependencies()
	for _, f := range info.MSIX.Dependencies.TargetDeviceFamilies {
		deps.AddTargetDeviceFamily(f.Name, f.MinVersion, f.MaxVersionTested)
	}
	return deps.Build()
}

func buildApplications(info *nfpm.Info) []msix.Application {
	apps := make([]msix.Application, 0, len(info.MSIX.Applications))
	for _, app := range info.MSIX.Applications {
		displayName := app.VisualElements.DisplayName
		if displayName == "" {
			displayName = info.Name
		}
		description := app.VisualElements.Description
		if description == "" {
			description = info.Description
		}
		ve := msix.NewVisualElements().
			WithDisplayName(displayName).
			WithDescription(description).
			WithBackgroundColor(app.VisualElements.BackgroundColor).
			WithSquare150x150Logo(app.VisualElements.Square150x150Logo).
			WithSquare44x44Logo(app.VisualElements.Square44x44Logo).
			Build()
		apps = append(apps, msix.NewApplication().
			WithID(app.ID).
			WithExecutable(app.Executable).
			WithEntryPoint(app.EntryPoint).
			WithVisualElements(ve).
			Build())
	}
	return apps
}

func buildCapabilities(info *nfpm.Info) msix.Capabilities {
	caps := msix.NewCapabilities()
	for _, c := range info.MSIX.Capabilities.Capabilities {
		caps.AddCapability(c)
	}
	for _, c := range info.MSIX.Capabilities.DeviceCapabilities {
		caps.AddDeviceCapability(msix.NewDeviceCapability().WithName(c).Build())
	}
	for _, c := range info.MSIX.Capabilities.Restricted {
		caps.AddRestricted(c)
	}
	return caps.Build()
}

func addContents(builder msix.Builder, info *nfpm.Info) {
	for _, content := range info.Contents {
		switch content.Type {
		case files.TypeDir:
			// Directories are implicit in MSIX — skip
			continue
		case files.TypeSymlink:
			log.Printf("warning: msix does not support symlinks, skipping %s", content.Destination)
			continue
		default:
			// Treat everything else (TypeFile, TypeConfig, etc.) as regular files.
			// AddFile defers errors to Build, so they surface from builder.Build.
			dest := normalizePathForMSIX(content.Destination)
			if content.Source != "" {
				builder.AddFile(dest, content.Source)
			}
		}
	}
}

func configureSigning(builder msix.Builder, info *nfpm.Info) error {
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

	builder.WithSigning(msix.NewSigning().
		WithCertificate(cert).
		WithPrivateKey(key).
		WithCertChain(chain...).
		Build())

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
