// Package ipk implements nfpm.Packager providing .ipk bindings.
//
// IPK is a package format used by the opkg package manager, which is very
// similar to the Debian package format.  Generally, the package format is
// stripped down and simplified compared to the Debian package format.
// Yocto/OpenEmbedded/OpenWRT uses the IPK format for its package management.
package ipk

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/deprecation"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/modtime"
)

const packagerName = "ipk"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// nolint: gochecknoglobals
var archToIPK = map[string]string{
	// all --> all
	"386":      "i386",
	"amd64":    "x86_64",
	"arm64":    "arm64",
	"arm5":     "armel",
	"arm6":     "armhf",
	"arm7":     "armhf",
	"mips64le": "mips64el",
	"mipsle":   "mipsel",
	"ppc64le":  "ppc64el",
	"s390":     "s390x",
}

func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	if info.IPK.Arch != "" {
		info.Arch = info.IPK.Arch
	} else if arch, ok := archToIPK[info.Arch]; ok {
		info.Arch = arch
	}

	return info
}

// Default ipk packager.
// nolint: gochecknoglobals
var Default = &IPK{}

// IPK is a ipk packager implementation.
type IPK struct{}

// ConventionalFileName returns a file name according
// to the conventions for ipk packages. Ipk packages generally follow
// the conventions set by debian.  See:
// https://manpages.debian.org/buster/dpkg-dev/dpkg-name.1.en.html
func (*IPK) ConventionalFileName(info *nfpm.Info) string {
	info = ensureValidArch(info)

	version := info.Version
	if info.Prerelease != "" {
		version += "~" + info.Prerelease
	}

	if info.VersionMetadata != "" {
		version += "+" + info.VersionMetadata
	}

	if info.Release != "" {
		version += "-" + info.Release
	}

	// package_version_architecture.package-type
	return fmt.Sprintf("%s_%s_%s.ipk", info.Name, version, info.Arch)
}

// ConventionalExtension returns the file name conventionally used for IPK packages
func (*IPK) ConventionalExtension() string {
	return ".ipk"
}

// SetPackagerDefaults sets the default values for the IPK packager.
func (*IPK) SetPackagerDefaults(info *nfpm.Info) {
	// Priority should be set on all packages per:
	//   https://www.debian.org/doc/debian-policy/ch-archive.html#priorities
	// "optional" seems to be the safe/sane default here
	if info.Priority == "" {
		info.Priority = "optional"
	}

	// The safe thing here feels like defaulting to something like below.
	// That will prevent existing configs from breaking anyway...  Wondering
	// if in the long run we should be more strict about this and error when
	// not set?
	if strings.TrimSpace(info.Maintainer) == "" {
		deprecation.Println("Leaving the 'maintainer' field unset will not be allowed in a future version")
		info.Maintainer = "Unset Maintainer <unset@localhost>"
	}
}

// Package writes a new ipk package to the given writer using the given info.
func (d *IPK) Package(info *nfpm.Info, ipk io.Writer) error {
	info = ensureValidArch(info)

	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}

	// Set up some ipk specific defaults
	d.SetPackagerDefaults(info)

	// Strip out any custom fields that are disallowed.
	stripDisallowedFields(info)

	contents, err := newTGZ("ipk",
		func(tw *tar.Writer) error {
			return createIPK(info, tw)
		},
	)
	if err != nil {
		return err
	}

	_, err = ipk.Write(contents)

	return err
}

// createIPK creates a new ipk package using the given tar writer and info.
func createIPK(info *nfpm.Info, ipk *tar.Writer) error {
	var installSize int64

	data, err := newTGZ("data.tar.gz",
		func(tw *tar.Writer) error {
			var err error
			installSize, err = populateDataTar(info, tw)
			return err
		},
	)
	if err != nil {
		return err
	}

	control, err := newTGZ("control.tar.gz",
		func(tw *tar.Writer) error {
			return populateControlTar(info, tw, installSize)
		},
	)
	if err != nil {
		return err
	}

	mtime := modtime.Get(info.MTime)

	if err := writeToFile(ipk, "debian-binary", []byte("2.0\n"), mtime); err != nil {
		return err
	}

	if err := writeToFile(ipk, "control.tar.gz", control, mtime); err != nil {
		return err
	}

	if err := writeToFile(ipk, "data.tar.gz", data, mtime); err != nil {
		return err
	}

	return nil
}

// populateDataTar populates the data tarball with the files specified in the info.
func populateDataTar(info *nfpm.Info, tw *tar.Writer) (instSize int64, err error) {
	// create files and implicit directories
	for _, file := range info.Contents {
		var size int64

		switch file.Type {
		case files.TypeDir, files.TypeImplicitDir:
			err = tw.WriteHeader(
				&tar.Header{
					Name:     files.AsExplicitRelativePath(file.Destination),
					Typeflag: tar.TypeDir,
					Format:   tar.FormatGNU,
					ModTime:  modtime.Get(info.MTime),
					Mode:     int64(file.FileInfo.Mode),
					Uname:    file.FileInfo.Owner,
					Gname:    file.FileInfo.Group,
				})
		case files.TypeSymlink:
			err = tw.WriteHeader(
				&tar.Header{
					Name:     files.AsExplicitRelativePath(file.Destination),
					Typeflag: tar.TypeSymlink,
					Format:   tar.FormatGNU,
					ModTime:  modtime.Get(info.MTime),
					Linkname: file.Source,
				})
		case files.TypeFile, files.TypeTree, files.TypeConfig, files.TypeConfigNoReplace:
			size, err = writeFile(tw, file)
		default:
			// ignore everything else
		}
		if err != nil {
			return 0, err
		}
		instSize += size
	}

	return instSize, nil
}

// getScripts returns the scripts for the given info.
func getScripts(info *nfpm.Info, mtime time.Time) []files.Content {
	return []files.Content{
		{
			Destination: "preinst",
			Source:      info.Scripts.PreInstall,
			FileInfo: &files.ContentFileInfo{
				Mode:  0o755,
				MTime: mtime,
			},
		}, {
			Destination: "postinst",
			Source:      info.Scripts.PostInstall,
			FileInfo: &files.ContentFileInfo{
				Mode:  0o755,
				MTime: mtime,
			},
		}, {
			Destination: "prerm",
			Source:      info.Scripts.PreRemove,
			FileInfo: &files.ContentFileInfo{
				Mode:  0o755,
				MTime: mtime,
			},
		}, {
			Destination: "postrm",
			Source:      info.Scripts.PostRemove,
			FileInfo: &files.ContentFileInfo{
				Mode:  0o755,
				MTime: mtime,
			},
		},
	}
}

// populateControlTar populates the control tarball with the control files defined
// in the info.
func populateControlTar(info *nfpm.Info, out *tar.Writer, instSize int64) error {
	var body bytes.Buffer

	cd := controlData{
		Info:          info,
		InstalledSize: instSize / 1024,
	}

	if err := renderControl(&body, cd); err != nil {
		return err
	}

	mtime := modtime.Get(info.MTime)
	if err := writeToFile(out, "./control", body.Bytes(), mtime); err != nil {
		return err
	}
	if err := writeToFile(out, "./conffiles", conffiles(info), mtime); err != nil {
		return err
	}

	scripts := getScripts(info, mtime)
	for _, file := range scripts {
		if file.Source != "" {
			if _, err := writeFile(out, &file); err != nil {
				return err
			}
		}
	}
	return nil
}

// conffiles returns the conffiles file bytes for the given info.
func conffiles(info *nfpm.Info) []byte {
	// nolint: prealloc
	var confs []string
	for _, file := range info.Contents {
		switch file.Type {
		case files.TypeConfig, files.TypeConfigNoReplace:
			confs = append(confs, files.NormalizeAbsoluteFilePath(file.Destination))
		}
	}
	return []byte(strings.Join(confs, "\n") + "\n")
}

// The ipk format is not formally defined, but it is similar to the deb format.
// The two sources that were used to create this template are:
// - https://git.yoctoproject.org/opkg/
// - https://github.com/openwrt/opkg-lede
//
// Supported Fields
//
// R = Required
// O = Optional
// e = Extra
// - = Not Supported/Ignored/Extra
//
//
//              OpenWRT   Yocto
//                    |   |
// | Field          | W | Y | Status |
// |----------------|---|---|--------|
// | ABIVersion     | O | - | ✓
// | Alternatives   | O | - | ✓
// | Architecture   | R | R | ✓
// | Auto-Installed | O | O | ✓
// | Conffiles      | O | O | not needed since config files are listed in .conffiles
// | Conflicts      | O | O | ✓
// | Depends        | R | R | ✓
// | Description    | R | R | ✓
// | Essential      | O | O | ✓
// | Filename       | - | - | an opkg field, not a package field
// | Homepage       | e | e | ✓
// | Installed-Size | O | O | ✓
// | Installed-Time | - | - | an opkg field, not a package field
// | License        | e | e | ✓
// | Maintainer     | R | R | ✓
// | MD5sum		    | - | - | insecure, not supported
// | Package        | R | R | ✓
// | Pre-Depends    | e | O | ✓
// | Priority       | R | R | ✓
// | Provides       | O | O | ✓
// | Recommends     | O | O | ✓
// | Replaces       | O | O | ✓
// | Section        | O | O | ✓
// | SHA256sum      | - | - | an opkg field, not a package field
// | Size           | - | - | an opkg field, not a package field
// | Source         | - | - | use the Fields field
// | Status		    | - | - | an opkg state, not a package field
// | Suggests       | O | O | ✓
// | Tags           | O | O | ✓
// | Vendor         | e | e | ✓
// | Version        | R | R | ✓
//
// If any values in user supplied Fields are found to be duplicates of the above
// fields, they will be stripped out.

// nolint: gochecknoglobals
var controlFields = []string{
	"ABIVersion",
	"Alternatives",
	"Architecture",
	"Auto-Installed",
	"Conffiles",
	"Conflicts",
	"Depends",
	"Description",
	"Essential",
	"Filename",
	"Homepage",
	"Installed-Size",
	"Installed-Time",
	"License",
	"Maintainer",
	"MD5sum",
	"Package",
	"Pre-Depends",
	"Priority",
	"Provides",
	"Recommends",
	"Replaces",
	"Section",
	"SHA256sum",
	"Size",
	// "Source", Allowed
	"Status",
	"Suggests",
	"Tags",
	"Vendor",
	"Version",
}

// stripDisallowedFields strips out any fields that are disallowed in the ipk
// format, ignoring case.
func stripDisallowedFields(info *nfpm.Info) {
	for key := range info.IPK.Fields {
		for _, disallowed := range controlFields {
			if strings.EqualFold(key, disallowed) {
				delete(info.IPK.Fields, key)
			}
		}
	}
}

const controlTemplate = `
{{- /* Mandatory fields */ -}}
Architecture: {{.Info.Arch}}
Description: {{multiline .Info.Description}}
Maintainer: {{.Info.Maintainer}}
Package: {{.Info.Name}}
Priority: {{.Info.Priority}}
Version: {{ if .Info.Epoch}}{{ .Info.Epoch }}:{{ end }}{{.Info.Version}}
         {{- if .Info.Prerelease}}~{{ .Info.Prerelease }}{{- end }}
         {{- if .Info.VersionMetadata}}+{{ .Info.VersionMetadata }}{{- end }}
         {{- if .Info.Release}}-{{ .Info.Release }}{{- end }}
{{- /* Optional fields */ -}}
{{- if .Info.IPK.ABIVersion}}
ABIVersion: {{.Info.IPK.ABIVersion}}
{{- end}}
{{- if .Info.IPK.Alternatives}}
Alternatives: {{ range $index, $element := .Info.IPK.Alternatives }}{{ if $index }}, {{end}}{{ $element.Priority }}:{{ $element.LinkName}}:{{ $element.Target}}{{- end }}
{{- end}}
{{- if .Info.IPK.AutoInstalled}}
Auto-Installed: yes
{{- end }}
{{- with .Info.Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- with .Info.Depends}}
Depends: {{join .}}
{{- end }}
{{- if .Info.IPK.Essential}}
Essential: yes
{{- end }}
{{- if .Info.Homepage}}
Homepage: {{.Info.Homepage}}
{{- end }}
{{- if .Info.License}}
License: {{.Info.License}}
{{- end }}
{{- if .InstalledSize }}
Installed-Size: {{.InstalledSize}}
{{- end }}
{{- with .Info.IPK.Predepends}}
Pre-Depends: {{join .}}
{{- end }}
{{- with nonEmpty .Info.Provides}}
Provides: {{join .}}
{{- end }}
{{- with .Info.Recommends}}
Recommends: {{join .}}
{{- end }}
{{- with .Info.Replaces}}
Replaces: {{join .}}
{{- end }}
{{- if .Info.Section}}
Section: {{.Info.Section}}
{{- end }}
{{- with .Info.Suggests}}
Suggests: {{join .}}
{{- end }}
{{- with .Info.IPK.Tags}}
Tags: {{join .}}
{{- end }}
{{- if .Info.Vendor}}
Vendor: {{.Info.Vendor}}
{{- end }}
{{- range $key, $value := .Info.IPK.Fields }}
{{- if $value }}
{{$key}}: {{$value}}
{{- end }}
{{- end }}
`

type controlData struct {
	Info          *nfpm.Info
	InstalledSize int64
}

func renderControl(w io.Writer, data controlData) error {
	tmpl := template.New("control")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"multiline": func(strs string) string {
			var b strings.Builder
			s := bufio.NewScanner(strings.NewReader(strings.TrimSpace(strs)))
			s.Scan()
			b.Write(bytes.TrimSpace(s.Bytes()))
			for s.Scan() {
				b.WriteString("\n ")
				l := bytes.TrimSpace(s.Bytes())
				if len(l) == 0 {
					b.WriteByte('.')
				} else {
					b.Write(l)
				}
			}
			return b.String()
		},
		"nonEmpty": func(strs []string) []string {
			var result []string
			for _, s := range strs {
				s := strings.TrimSpace(s)
				if s == "" {
					continue
				}
				result = append(result, s)
			}
			return result
		},
	})
	return template.Must(tmpl.Parse(controlTemplate)).Execute(w, data)
}
