// Package nfpm provides ways to package programs in some linux packaging
// formats.
package nfpm

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"

	yaml "gopkg.in/yaml.v2"
)

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
	err = dec.Decode(&config)
	return
}

// ParseFile decodes YAML data from a file path into a configuration struct
func ParseFile(path string) (config Config, err error) {
	var file *os.File
	file, err = os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	return Parse(file)
}

// Packager represents any packager implementation
type Packager interface {
	Package(info Info, w io.Writer) error
}

// Config contains the top level configuration for packages
type Config struct {
	Info      `yaml:",inline"`
	Overrides map[string]interface{} `yaml:"overrides,omitempty"`
}

// Get returns the Info struct for the given packager format. Overrides
// for the given format are merged into the final struct
func (c Config) Get(format string) (info Info, err error) {
	if err = mergo.Merge(&info, c.Info); err != nil {
		return
	}
	data, ok := c.Overrides[format]
	if !ok {
		// no overrides
		return
	}
	var override Info
	config := &mapstructure.DecoderConfig{
		Result:  &override,
		TagName: "yaml",
	}
	var dec *mapstructure.Decoder
	if dec, err = mapstructure.NewDecoder(config); err != nil {
		return
	}
	if err = dec.Decode(data); err != nil {
		return
	}
	if err = mergo.Merge(&info, override, mergo.WithOverride); err != nil {
		return
	}
	return
}

// Info contains information about a single package
type Info struct {
	Name        string            `yaml:"name,omitempty"`
	Arch        string            `yaml:"arch,omitempty"`
	Platform    string            `yaml:"platform,omitempty"`
	Version     string            `yaml:"version,omitempty"`
	Section     string            `yaml:"section,omitempty"`
	Priority    string            `yaml:"priority,omitempty"`
	Replaces    []string          `yaml:"replaces,omitempty"`
	Provides    []string          `yaml:"provides,omitempty"`
	Depends     []string          `yaml:"depends,omitempty"`
	Recommends  []string          `yaml:"recommends,omitempty"`
	Suggests    []string          `yaml:"suggests,omitempty"`
	Conflicts   []string          `yaml:"conflicts,omitempty"`
	Maintainer  string            `yaml:"maintainer,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Vendor      string            `yaml:"vendor,omitempty"`
	Homepage    string            `yaml:"homepage,omitempty"`
	License     string            `yaml:"license,omitempty"`
	Bindir      string            `yaml:"bindir,omitempty"`
	Files       map[string]string `yaml:"files,omitempty"`
	ConfigFiles map[string]string `yaml:"config_files,omitempty"`
	Scripts     Scripts           `yaml:"scripts,omitempty"`
}

// Scripts contains information about maintainer scripts for packages
type Scripts struct {
	PreInstall  string `yaml:"preinstall,omitempty"`
	PostInstall string `yaml:"postinstall,omitempty"`
	PreRemove   string `yaml:"preremove,omitempty"`
	PostRemove  string `yaml:"postremove,omitempty"`
}

// Validate the given Info and returns an error if it is invalid.
func Validate(info Info) error {
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
func WithDefaults(info Info) Info {
	if info.Bindir == "" {
		info.Bindir = "/usr/local/bin"
	}
	if info.Platform == "" {
		info.Platform = "linux"
	}
	if info.Description == "" {
		info.Description = "no description given"
	}
	info.Version = strings.TrimPrefix(info.Version, "v")
	return info
}
