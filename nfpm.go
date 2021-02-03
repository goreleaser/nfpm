// Package nfpm provides ways to package programs in some linux packaging
// formats.
package nfpm

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/AlekSi/pointer"
	"github.com/Masterminds/semver/v3"
	"github.com/goreleaser/chglog"
	"github.com/imdario/mergo"
	"gopkg.in/yaml.v3"

	"github.com/goreleaser/nfpm/v2/files"
)

// nolint: gochecknoglobals
var (
	packagers = map[string]Packager{}
	lock      sync.Mutex
)

// RegisterPackager a new packager for the given format.
func RegisterPackager(format string, p Packager) {
	lock.Lock()
	defer lock.Unlock()
	packagers[format] = p
}

// ClearPackagers clear all registered packagers, used for testing.
func ClearPackagers() {
	lock.Lock()
	defer lock.Unlock()
	packagers = map[string]Packager{}
}

// ErrNoPackager happens when no packager is registered for the given format.
type ErrNoPackager struct {
	format string
}

func (e ErrNoPackager) Error() string {
	return fmt.Sprintf("no packager registered for the format %s", e.format)
}

// Get a packager for the given format.
func Get(format string) (Packager, error) {
	p, ok := packagers[format]
	if !ok {
		return nil, ErrNoPackager{format}
	}
	return p, nil
}

// Parse decodes YAML data from an io.Reader into a configuration struct.
func Parse(in io.Reader) (config Config, err error) {
	return ParseWithEnvMapping(in, os.Getenv)
}

// ParseWithEnvMapping decodes YAML data from an io.Reader into a configuration struct.
func ParseWithEnvMapping(in io.Reader, mapping func(string) string) (config Config, err error) {
	dec := yaml.NewDecoder(in)
	dec.KnownFields(true)
	if err = dec.Decode(&config); err != nil {
		return
	}
	config.envMappingFunc = mapping
	if config.envMappingFunc == nil {
		config.envMappingFunc = func(s string) string { return s }
	}

	config.expandEnvVars()

	WithDefaults(&config.Info)

	return config, config.Validate()
}

// ParseFile decodes YAML data from a file path into a configuration struct.
func ParseFile(path string) (config Config, err error) {
	return ParseFileWithEnvMapping(path, os.Getenv)
}

// ParseFileWithEnvMapping decodes YAML data from a file path into a configuration struct.
func ParseFileWithEnvMapping(path string, mapping func(string) string) (config Config, err error) {
	var file *os.File
	file, err = os.Open(path) //nolint:gosec
	if err != nil {
		return
	}
	defer file.Close() // nolint: errcheck,gosec
	return ParseWithEnvMapping(file, mapping)
}

// Packager represents any packager implementation.
type Packager interface {
	Package(info *Info, w io.Writer) error
	ConventionalFileName(info *Info) string
}

// Config contains the top level configuration for packages.
type Config struct {
	Info           `yaml:",inline"`
	Overrides      map[string]Overridables `yaml:"overrides,omitempty"`
	envMappingFunc func(string) string
}

// Get returns the Info struct for the given packager format. Overrides
// for the given format are merged into the final struct.
func (c *Config) Get(format string) (info *Info, err error) {
	info = &Info{}
	// make a deep copy of info
	if err = mergo.Merge(info, c.Info); err != nil {
		return nil, fmt.Errorf("failed to merge config into info: %w", err)
	}
	override, ok := c.Overrides[format]
	if !ok {
		// no overrides
		return info, nil
	}
	if err = mergo.Merge(&info.Overridables, override, mergo.WithOverride); err != nil {
		return nil, fmt.Errorf("failed to merge overrides into info: %w", err)
	}
	var contents []*files.Content
	for _, f := range info.Contents {
		if f.Packager == format || f.Packager == "" {
			contents = append(contents, f)
		}
	}
	info.Contents = contents
	return info, nil
}

// Validate ensures that the config is well typed.
func (c *Config) Validate() error {
	if err := Validate(&c.Info); err != nil {
		return err
	}
	for format := range c.Overrides {
		if _, err := Get(format); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) expandEnvVars() {
	// Version related fields
	c.Info.Release = os.Expand(c.Info.Release, c.envMappingFunc)
	c.Info.Version = os.Expand(c.Info.Version, c.envMappingFunc)
	c.Info.Prerelease = os.Expand(c.Info.Prerelease, c.envMappingFunc)
	c.Info.Arch = os.Expand(c.Info.Arch, c.envMappingFunc)

	// Package signing related fields
	c.Info.Deb.Signature.KeyFile = os.Expand(c.Deb.Signature.KeyFile, c.envMappingFunc)
	c.Info.RPM.Signature.KeyFile = os.Expand(c.RPM.Signature.KeyFile, c.envMappingFunc)
	c.Info.APK.Signature.KeyFile = os.Expand(c.APK.Signature.KeyFile, c.envMappingFunc)
	c.Info.Deb.Signature.KeyID = pointer.ToString(os.Expand(pointer.GetString(c.Deb.Signature.KeyID), c.envMappingFunc))
	c.Info.RPM.Signature.KeyID = pointer.ToString(os.Expand(pointer.GetString(c.RPM.Signature.KeyID), c.envMappingFunc))
	c.Info.APK.Signature.KeyID = pointer.ToString(os.Expand(pointer.GetString(c.APK.Signature.KeyID), c.envMappingFunc))

	// Package signing passphrase
	generalPassphrase := os.Expand("$NFPM_PASSPHRASE", c.envMappingFunc)
	c.Info.Deb.Signature.KeyPassphrase = generalPassphrase
	c.Info.RPM.Signature.KeyPassphrase = generalPassphrase
	c.Info.APK.Signature.KeyPassphrase = generalPassphrase

	debPassphrase := os.Expand("$NFPM_DEB_PASSPHRASE", c.envMappingFunc)
	if debPassphrase != "" {
		c.Info.Deb.Signature.KeyPassphrase = debPassphrase
	}

	rpmPassphrase := os.Expand("$NFPM_RPM_PASSPHRASE", c.envMappingFunc)
	if rpmPassphrase != "" {
		c.Info.RPM.Signature.KeyPassphrase = rpmPassphrase
	}

	apkPassphrase := os.Expand("$NFPM_APK_PASSPHRASE", c.envMappingFunc)
	if apkPassphrase != "" {
		c.Info.APK.Signature.KeyPassphrase = apkPassphrase
	}
}

// Info contains information about a single package.
type Info struct {
	Overridables    `yaml:",inline"`
	Name            string `yaml:"name,omitempty"`
	Arch            string `yaml:"arch,omitempty"`
	Platform        string `yaml:"platform,omitempty"`
	Epoch           string `yaml:"epoch,omitempty"`
	Version         string `yaml:"version,omitempty"`
	Release         string `yaml:"release,omitempty"`
	Prerelease      string `yaml:"prerelease,omitempty"`
	VersionMetadata string `yaml:"version_metadata,omitempty"`
	Section         string `yaml:"section,omitempty"`
	Priority        string `yaml:"priority,omitempty"`
	Maintainer      string `yaml:"maintainer,omitempty"`
	Description     string `yaml:"description,omitempty"`
	Vendor          string `yaml:"vendor,omitempty"`
	Homepage        string `yaml:"homepage,omitempty"`
	License         string `yaml:"license,omitempty"`
	Changelog       string `yaml:"changelog,omitempty"`
	DisableGlobbing bool   `yaml:"disable_globbing"`
	Target          string `yaml:"-"`
}

func (i *Info) Validate() error {
	return Validate(i)
}

// GetChangeLog parses the provided changelog file.
func (i *Info) GetChangeLog() (log *chglog.PackageChangeLog, err error) {
	// if the file does not exist chglog.Parse will just silently
	// create an empty changelog but we should notify the user instead
	if _, err = os.Stat(i.Changelog); os.IsNotExist(err) {
		return nil, err
	}

	entries, err := chglog.Parse(i.Changelog)
	if err != nil {
		return nil, err
	}

	return &chglog.PackageChangeLog{
		Name:    i.Name,
		Entries: entries,
	}, nil
}

// Overridables contain the field which are overridable in a package.
type Overridables struct {
	Replaces     []string       `yaml:"replaces,omitempty"`
	Provides     []string       `yaml:"provides,omitempty"`
	Depends      []string       `yaml:"depends,omitempty"`
	Recommends   []string       `yaml:"recommends,omitempty"`
	Suggests     []string       `yaml:"suggests,omitempty"`
	Conflicts    []string       `yaml:"conflicts,omitempty"`
	Contents     files.Contents `yaml:"contents,omitempty"`
	EmptyFolders []string       `yaml:"empty_folders,omitempty"`
	Scripts      Scripts        `yaml:"scripts,omitempty"`
	RPM          RPM            `yaml:"rpm,omitempty"`
	Deb          Deb            `yaml:"deb,omitempty"`
	APK          APK            `yaml:"apk,omitempty"`
}

// RPM is custom configs that are only available on RPM packages.
type RPM struct {
	Group       string       `yaml:"group,omitempty"`
	Summary     string       `yaml:"summary,omitempty"`
	Compression string       `yaml:"compression,omitempty"`
	Signature   RPMSignature `yaml:"signature,omitempty"`
}

type PackageSignature struct {
	// PGP secret key, can be ASCII-armored
	KeyFile       string  `yaml:"key_file,omitempty"`
	KeyID         *string `yaml:"key_id,omitempty"`
	KeyPassphrase string  `yaml:"-"` // populated from environment variable
}

type RPMSignature struct {
	PackageSignature `yaml:",inline"`
}

type APK struct {
	Signature APKSignature `yaml:"signature,omitempty"`
}

type APKSignature struct {
	PackageSignature `yaml:",inline"`
	// defaults to <maintainer email>.rsa.pub
	KeyName string `yaml:"key_name,omitempty"`
}

// Deb is custom configs that are only available on deb packages.
type Deb struct {
	Scripts   DebScripts   `yaml:"scripts,omitempty"`
	Triggers  DebTriggers  `yaml:"triggers,omitempty"`
	Breaks    []string     `yaml:"breaks,omitempty"`
	Signature DebSignature `yaml:"signature,omitempty"`
}

type DebSignature struct {
	PackageSignature `yaml:",inline"`
	// origin, maint or archive (defaults to origin)
	Type string `yaml:"type,omitempty"`
}

// DebTriggers contains triggers only available for deb packages.
// https://wiki.debian.org/DpkgTriggers
// https://man7.org/linux/man-pages/man5/deb-triggers.5.html
type DebTriggers struct {
	Interest        []string `yaml:"interest,omitempty"`
	InterestAwait   []string `yaml:"interest_await,omitempty"`
	InterestNoAwait []string `yaml:"interest_noawait,omitempty"`
	Activate        []string `yaml:"activate,omitempty"`
	ActivateAwait   []string `yaml:"activate_await,omitempty"`
	ActivateNoAwait []string `yaml:"activate_noawait,omitempty"`
}

// DebScripts is scripts only available on deb packages.
type DebScripts struct {
	Rules     string `yaml:"rules,omitempty"`
	Templates string `yaml:"templates,omitempty"`
	Config    string `yaml:"config,omitempty"`
}

// Scripts contains information about maintainer scripts for packages.
type Scripts struct {
	PreInstall  string `yaml:"preinstall,omitempty"`
	PostInstall string `yaml:"postinstall,omitempty"`
	PreRemove   string `yaml:"preremove,omitempty"`
	PostRemove  string `yaml:"postremove,omitempty"`
}

// ErrFieldEmpty happens when some required field is empty.
type ErrFieldEmpty struct {
	field string
}

func (e ErrFieldEmpty) Error() string {
	return fmt.Sprintf("package %s must be provided", e.field)
}

// Validate the given Info and returns an error if it is invalid.
func Validate(info *Info) (err error) {
	if info.Name == "" {
		return ErrFieldEmpty{"name"}
	}
	if info.Arch == "" {
		return ErrFieldEmpty{"arch"}
	}
	if info.Version == "" {
		return ErrFieldEmpty{"version"}
	}

	contents, err := files.ExpandContentGlobs(info.Contents, info.DisableGlobbing)
	if err != nil {
		return err
	}

	info.Contents = contents
	return nil
}

// WithDefaults set some sane defaults into the given Info.
func WithDefaults(info *Info) *Info {
	if info.Platform == "" {
		info.Platform = "linux"
	}
	if info.Description == "" {
		info.Description = "no description given"
	}
	if info.Arch == "" {
		info.Arch = "amd64"
	}
	if info.Version == "" {
		info.Version = "v0.0.0-rc0"
	}

	// parse the version as a semver so we can properly split the parts
	// and support proper ordering for both rpm and deb
	if v, err := semver.NewVersion(info.Version); err == nil {
		info.Version = fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch())
		if info.Prerelease == "" {
			info.Prerelease = v.Prerelease()
		}

		if info.VersionMetadata == "" {
			info.VersionMetadata = v.Metadata()
		}
	}

	return info
}

// ErrSigningFailure is returned whenever something went wrong during
// the package signing process. The underlying error can be unwrapped
// and could be crypto-related or something that occurred while adding
// the signature to the package.
type ErrSigningFailure struct {
	Err error
}

func (s *ErrSigningFailure) Error() string {
	return fmt.Sprintf("signing error: %v", s.Err)
}

func (s *ErrSigningFailure) Unwarp() error {
	return s.Err
}
