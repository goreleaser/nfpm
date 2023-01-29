package deb

import (
	"github.com/goreleaser/nfpm/v2"
	"io"
	"strings"
)

const debChangesTemplate = `
{{- /* Mandatory fields */ -}}
Format: 1.8
Date: {{.Date}}
Source: {{.Info.Name}}
Architecture: {{ if ne .Info.Platform "linux"}}{{ .Info.Platform }}-{{ end }}{{.Info.Arch}}
Version: {{ if .Info.Epoch}}{{ .Info.Epoch }}:{{ end }}{{.Info.Version}}
         {{- if .Info.Prerelease}}~{{ .Info.Prerelease }}{{- end }}
         {{- if .Info.VersionMetadata}}+{{ .Info.VersionMetadata }}{{- end }}
         {{- if .Info.Release}}-{{ .Info.Release }}{{- end }}
Distribution: {{ .Info.Deb.Distribution }}
{{- if .Info.Deb.Urgency}}
Urgency: {{.Info.Deb.Urgency}}
{{- end }}
Maintainer: {{.Info.Maintainer}}
Description: {{multiline .Info.Description}}
{{- /* Optional fields */ -}}
{{- range $key, $value := .Info.Deb.Fields }}
{{- if $value }}
{{$key}}: {{$value}}
{{- end }}
{{- /* Mandatory fields */ -}}
Changes:
{{range .Changes}} {{.}}{{end}}
Files:
{{range .Files}} {{ .Md5Sum }} {{.Size}} {{.Section}} {{.Priority}} {{.Name}}{{end}}
Checksums-Sha1:
{{range .Files}} {{ .Sha1Sum }} {{.Size}} {{.Name}}{{end}}
Checksums-Sha256:
{{range .Files}} {{ .Sha256Sum }} {{.Size}} {{.Name}}{{end}}
`

type changesData struct {
	Info    *nfpm.Info
	Changes []string
	Files   []changesFileData
}

type changesFileData struct {
	Name      string
	Size      int
	Section   string
	Priority  string
	Md5Sum    string
	Sha1Sum   string
	Sha256Sum string
}

func (d *Deb) ConventionalMetadataFileName(info *nfpm.Info) string {
	target := info.Target

	if target == "" {
		target = d.ConventionalFileName(info)
	}

	return strings.Replace(target, ".deb", ".changes", 1)
}

func (d *Deb) PackageMetadata(info *nfpm.Info, changes io.Writer) error {
	info = ensureValidArch(info)

	if err := info.Validate(); err != nil {
		return err
	}

	// Set up some deb specific defaults
	d.SetPackagerDefaults(info)

	if err := d.createChanges(info, changes); err != nil {
		return err
	}

	// todo:
	// 2. Prepare changes template data (changes, files & checksums)
	// 3. Render template
	// 4. Sign if required

	return nil
}

func (d *Deb) createChanges(info *nfpm.Info, w io.Writer) error {
	_ = d.prepareChangesValues(info)

	return nil
}

func (d *Deb) prepareChangesValues(info *nfpm.Info) *changesData {
	return &changesData{}
}
