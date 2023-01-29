package deb

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"github.com/goreleaser/nfpm/v2"
	"io"
	"path/filepath"
	"strings"
	"time"
)

const debChangesTemplate = `
{{- /* Mandatory fields */ -}}
Format: 1.8
Date: {{.Date}}
Source: {{.Info.Name}}
Binary: {{.Info.Name}}
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
Description: {{ multiline .Info.Description }}
{{- range $key, $value := .Info.Deb.Fields }}
{{- if $value }}
{{$key}}: {{$value}}
{{- end }}
{{- end }}
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
	Date    string
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

func (d *Deb) PackageMetadata(info *nfpm.MetaInfo, w io.Writer) error {
	if err := createChanges(info, w); err != nil {
		return err
	}

	// todo: Sign if required

	return nil
}

func createChanges(info *nfpm.MetaInfo, w io.Writer) error {
	data, err := prepareChangesData(info)
	if err != nil {
		return err
	}

	return writeTemplate("changes", debChangesTemplate, w, data)
}

func prepareChangesData(meta *nfpm.MetaInfo) (*changesData, error) {
	info := meta.Info

	_, err := meta.Package.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)

	_, err = buf.ReadFrom(meta.Package)
	if err != nil {
		return nil, err
	}

	return &changesData{
		Date: time.Now().Format(time.RFC1123Z),
		Info: info,
		Changes: []string{
			fmt.Sprintf("%s (%s) %s; urgency=%s\n  *Package created with nFPM",
				info.Name, info.Version, info.Deb.Distribution, info.Deb.Urgency),
		},
		Files: []changesFileData{
			{
				Name:      filepath.Base(info.Target),
				Size:      buf.Len(),
				Section:   "default",
				Priority:  "optional",
				Md5Sum:    fmt.Sprintf("%x", md5.Sum(buf.Bytes())),
				Sha1Sum:   fmt.Sprintf("%x", sha1.Sum(buf.Bytes())),
				Sha256Sum: fmt.Sprintf("%x", sha256.Sum256(buf.Bytes())),
			},
		},
	}, nil
}
