// Package nfpm provides ways to package programs in some linux packaging
// formats.
package nfpm

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/goreleaser/chglog"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	gitConfig "gopkg.in/src-d/go-git.v4/config"

	"gopkg.in/yaml.v2"
)

// nolint: gochecknoglobals
var (
	packagers = map[string]Packager{}
	lock      sync.Mutex
)

// Register a new packager for the given format
func Register(format string, p Packager) {
	lock.Lock()
	packagers[format] = p
	lock.Unlock()
}

// Get a packager for the given format
func Get(format string) (Packager, error) {
	p, ok := packagers[format]
	if !ok {
		return nil, fmt.Errorf("no packager registered for the format %s", format)
	}
	return p, nil
}

// Parse decodes YAML data from an io.Reader into a configuration struct
func Parse(in io.Reader) (config Config, err error) {
	dec := yaml.NewDecoder(in)
	dec.SetStrict(true)
	if err = dec.Decode(&config); err != nil {
		return
	}

	config.Info.Release = os.ExpandEnv(config.Info.Release)
	config.Info.Version = os.ExpandEnv(config.Info.Version)

	return config, config.Validate()
}

// ParseFile decodes YAML data from a file path into a configuration struct
func ParseFile(path string) (config Config, err error) {
	var file *os.File
	file, err = os.Open(path) //nolint:gosec
	if err != nil {
		return
	}
	defer file.Close() // nolint: errcheck
	return Parse(file)
}

// Packager represents any packager implementation
type Packager interface {
	Package(info *Info, w io.Writer) error
}

// Config contains the top level configuration for packages
type Config struct {
	Info      `yaml:",inline"`
	Overrides map[string]Overridables `yaml:"overrides,omitempty"`
}

// Get returns the Info struct for the given packager format. Overrides
// for the given format are merged into the final struct
func (c *Config) Get(format string) (info *Info, err error) {
	info = &Info{
		semver: c.Info.semver,
	}
	// make a deep copy of info
	if err = mergo.Merge(info, c.Info); err != nil {
		return nil, errors.Wrap(err, "failed to merge config into info")
	}
	override, ok := c.Overrides[format]
	if !ok {
		// no overrides
		return info, nil
	}
	if err = mergo.Merge(&info.Overridables, override, mergo.WithOverride); err != nil {
		return nil, errors.Wrap(err, "failed to merge overrides into info")
	}
	return info, nil
}

// Validate ensures that the config is well typed
func (c *Config) Validate() error {
	for format := range c.Overrides {
		if _, err := Get(format); err != nil {
			return err
		}
	}
	return nil
}

// Info contains information about a single package
type Info struct {
	Overridables `yaml:",inline"`
	Name         string                   `yaml:"name,omitempty"`
	Arch         string                   `yaml:"arch,omitempty"`
	Platform     string                   `yaml:"platform,omitempty"`
	Epoch        string                   `yaml:"epoch,omitempty"`
	Version      string                   `yaml:"version,omitempty"`
	Release      string                   `yaml:"release,omitempty"`
	Section      string                   `yaml:"section,omitempty"`
	Priority     string                   `yaml:"priority,omitempty"`
	Maintainer   string                   `yaml:"maintainer,omitempty"`
	Description  string                   `yaml:"description,omitempty"`
	Vendor       string                   `yaml:"vendor,omitempty"`
	Homepage     string                   `yaml:"homepage,omitempty"`
	License      string                   `yaml:"license,omitempty"`
	Bindir       string                   `yaml:"bindir,omitempty"`
	Chglog       string                   `yaml:"chglog,omitempty"`
	Target       string                   `yaml:"-"`
	chglogData   *chglog.PackageChangeLog `yaml:"-"`
	semver       *semver.Version          `yaml:"-"`
}

// Overridables contain the field which are overridable in a package
type Overridables struct {
	Replaces     []string          `yaml:"replaces,omitempty"`
	Provides     []string          `yaml:"provides,omitempty"`
	Depends      []string          `yaml:"depends,omitempty"`
	Recommends   []string          `yaml:"recommends,omitempty"`
	Suggests     []string          `yaml:"suggests,omitempty"`
	Conflicts    []string          `yaml:"conflicts,omitempty"`
	Files        map[string]string `yaml:"files,omitempty"`
	ConfigFiles  map[string]string `yaml:"config_files,omitempty"`
	EmptyFolders []string          `yaml:"empty_folders,omitempty"`
	Scripts      Scripts           `yaml:"scripts,omitempty"`
	RPM          RPM               `yaml:"rpm,omitempty"`
	Deb          Deb               `yaml:"deb,omitempty"`
}

// RPM is custom configs that are only available on RPM packages
type RPM struct {
	Group       string `yaml:"group,omitempty"`
	Compression string `yaml:"compression,omitempty"`
}

// Deb is custom configs that are only available on deb packages
type Deb struct {
	Scripts         DebScripts `yaml:"scripts,omitempty"`
	VersionMetadata string     `yaml:"metadata,omitempty"`
	Distribution    []string   `yaml:"distribution,omitempty"`
	Urgency         string     `yaml:"urgency,omitempty"`
}

// DebScripts is scripts only available on deb packages
type DebScripts struct {
	Rules string `yaml:"rules,omitempty"`
}

// Scripts contains information about maintainer scripts for packages
type Scripts struct {
	PreInstall  string `yaml:"preinstall,omitempty"`
	PostInstall string `yaml:"postinstall,omitempty"`
	PreRemove   string `yaml:"preremove,omitempty"`
	PostRemove  string `yaml:"postremove,omitempty"`
}

// Validate the given Info and returns an error if it is invalid.
func Validate(info *Info) error {
	if info.Name == "" {
		return fmt.Errorf("package name cannot be empty")
	}
	if info.Arch == "" {
		return fmt.Errorf("package arch must be provided")
	}
	if info.Version == "" {
		return fmt.Errorf("package version must be provided")
	}
	if len(info.Files)+len(info.ConfigFiles) == 0 {
		return fmt.Errorf("no files were provided")
	}
	return nil
}

// WithDefaults set some sane defaults into the given Info
func WithDefaults(info *Info) *Info {
	if info.Bindir == "" {
		info.Bindir = "/usr/local/bin"
	}
	if info.Platform == "" {
		info.Platform = "linux"
	}
	if info.Description == "" {
		info.Description = "no description given"
	}
	if info.Chglog == "" {
		info.Chglog = "changelog.yml"
	}
	if info.Deb.Urgency == "" {
		info.Deb.Urgency = "low"
	}
	if len(info.Deb.Distribution) == 0 {
		info.Deb.Distribution = append(info.Deb.Distribution, "unstable")
	}
	if info.Maintainer == "" {
		info.Maintainer = "UNKNOWN <UNKNOWN>"
		var (
			repo *git.Repository
			err  error
			cwd  string
		)
		if cwd, err = os.Getwd(); err == nil {
			if repo, err = chglog.GitRepo(cwd, true); err == nil {
				var repoCfg *gitConfig.Config
				if repoCfg, err = repo.Config(); err == nil {
					info.Maintainer = fmt.Sprintf("%s <%s>", repoCfg.User.Name, repoCfg.User.Email)
				}
			}
		}
		if err != nil {
			fmt.Println(err, cwd)
		}
	}

	// parse the version as a semver so we can properly split the parts
	// and support proper ordering for both rpm and deb
	if v, err := semver.NewVersion(info.Version); err == nil {
		info.Version = fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch())
		if info.Release == "" {
			// maybe we should concat the previous info.Release with v.Prerelease?
			info.Release = v.Prerelease()
		}
		info.Deb.VersionMetadata = v.Metadata()
		info.semver = v
	}
	return info
}

// GetChangeLog Get the properly formatted changelog data
func (i *Info) GetChangeLog() (log *chglog.PackageChangeLog, err error) {
	if i.chglogData != nil {
		return i.chglogData, nil
	}

	now := time.Now()
	blankEntry := &chglog.ChangeLog{
		Semver:   i.semver.String(),
		Date:     now,
		Packager: i.Maintainer,
		Changes: chglog.ChangeLogChanges{
			&chglog.ChangeLogChange{
				Note: fmt.Sprintf("packaged on %s", now),
			},
		},
	}
	blankEntry.Deb = &chglog.ChangelogDeb{Urgency: i.Deb.Urgency, Distributions: i.Deb.Distribution}
	if !strings.Contains(blankEntry.Packager, "<") && !strings.Contains(blankEntry.Packager, ">") {
		blankEntry.Packager = fmt.Sprintf("%s <%s>", blankEntry.Packager, blankEntry.Packager)
	}
	i.chglogData = &chglog.PackageChangeLog{
		Name:    i.Name,
		Entries: chglog.ChangeLogEntries{blankEntry},
	}

	if i.chglogData.Entries, err = chglog.Parse(i.Chglog); err != nil {
		i.chglogData = nil
		return nil, err
	}
	if len(i.chglogData.Entries) == 0 {
		i.chglogData.Entries = append(i.chglogData.Entries, blankEntry)
	}

	return i.chglogData, nil
}
