package deb

import (
	"bytes"
	"crypto/md5"  //nolint:gosec // required by the .changes format
	"crypto/sha1" //nolint:gosec // required by the .changes format
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/internal/sign"
)

// https://www.debian.org/doc/debian-policy/ch-controlfields.html#debian-changes-files-changes
const debChangesTemplate = `
{{- /* Mandatory fields */ -}}
Format: 1.8
Date: {{.Date}}
Source: {{.Info.Name}}
Binary: {{.Info.Name}}
Architecture: {{ if ne .Info.Platform "linux"}}{{ .Info.Platform }}-{{ end }}{{.Info.Arch}}
Version: {{.Version}}
Distribution: {{.Info.Deb.Metadata.Distribution}}
Urgency: {{.Info.Deb.Metadata.Urgency}}
Maintainer: {{.Info.Maintainer}}
{{- if .Info.Deb.Metadata.ChangedBy }}
Changed-By: {{.Info.Deb.Metadata.ChangedBy}}
{{- end }}
Description:
 {{.Info.Name}}{{ if .Synopsis }} - {{.Synopsis}}{{ end }}
{{- range $key, $value := .Info.Deb.Metadata.Fields }}
{{- if $value }}
{{$key}}: {{$value}}
{{- end }}
{{- end }}
Changes:
{{range .Changes}} {{.}}{{end}}
Checksums-Sha256:
{{range .Files}} {{ .Sha256Sum }} {{.Size}} {{.Name}}{{end}}
Checksums-Sha1:
{{range .Files}} {{ .Sha1Sum }} {{.Size}} {{.Name}}{{end}}
Files:
{{range .Files}} {{ .Md5Sum }} {{.Size}} {{.Section}} {{.Priority}} {{.Name}}{{end}}

`

type changesData struct {
	Date     string
	Info     *nfpm.Info
	Version  string
	Synopsis string
	Changes  []string
	Files    []changesFileData
}

type changesFileData struct {
	Name      string
	Size      int64
	Section   string
	Priority  string
	Md5Sum    string
	Sha1Sum   string
	Sha256Sum string
}

var now = time.Now

// ConventionalMetadataFileName returns the name of the .changes file for the
// given info. It is always a different file than the package itself.
func (d *Deb) ConventionalMetadataFileName(info *nfpm.Info) string {
	target := info.Target

	if target == "" {
		target = d.ConventionalFileName(info)
	}

	return strings.TrimSuffix(target, ".deb") + ".changes"
}

// PackageMetadata writes the .changes file of the package in metaInfo.Package.
func (d *Deb) PackageMetadata(metaInfo *nfpm.MetaInfo, w io.Writer) error {
	if err := d.validateChanges(metaInfo.Info); err != nil {
		return err
	}

	data, err := d.createChanges(metaInfo)
	if err != nil {
		return err
	}

	sig := metaInfo.Info.Deb.Signature
	if sig.KeyFile == "" && sig.SignFn == nil {
		_, err = w.Write(data)
		return err
	}

	signedData, err := signChanges(data, metaInfo.Info)
	if err != nil {
		return err
	}

	_, err = w.Write(signedData)
	return err
}

func (d *Deb) validateChanges(info *nfpm.Info) error {
	if info.Deb.Metadata.Distribution == "" {
		return errors.New("deb.metadata.distribution is required when enable_metadata is set")
	}

	if info.Deb.Metadata.Urgency == "" {
		return errors.New("deb.metadata.urgency is required when enable_metadata is set")
	}

	// upload tools like reprepro parse name, version and architecture from the file name.
	if name, expected := d.packageFileName(info), d.ConventionalFileName(info); name != expected {
		return fmt.Errorf("package file name %q is not supported when enable_metadata is set, use %q", name, expected)
	}

	return nil
}

func (d *Deb) packageFileName(info *nfpm.Info) string {
	if info.Target == "" {
		return d.ConventionalFileName(info)
	}

	return filepath.Base(info.Target)
}

func (d *Deb) createChanges(meta *nfpm.MetaInfo) ([]byte, error) {
	data, err := d.prepareChangesData(meta)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeTemplate("changes", debChangesTemplate, &buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (d *Deb) prepareChangesData(meta *nfpm.MetaInfo) (*changesData, error) {
	info := meta.Info

	if _, err := meta.Package.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	md5Hash, sha1Hash, sha256Hash := md5.New(), sha1.New(), sha256.New() //nolint:gosec
	size, err := io.Copy(io.MultiWriter(md5Hash, sha1Hash, sha256Hash), meta.Package)
	if err != nil {
		return nil, err
	}

	version := changesVersion(info)
	synopsis, _, _ := strings.Cut(strings.TrimSpace(info.Description), "\n")

	return &changesData{
		Date:     now().Format(time.RFC1123Z),
		Info:     info,
		Version:  version,
		Synopsis: strings.TrimSpace(synopsis),
		Changes: []string{
			fmt.Sprintf("%s (%s) %s; urgency=%s\n  * Package created with nFPM",
				info.Name, version, info.Deb.Metadata.Distribution, info.Deb.Metadata.Urgency),
		},
		Files: []changesFileData{
			{
				Name:      d.packageFileName(info),
				Size:      size,
				Section:   dashIfEmpty(info.Section),
				Priority:  dashIfEmpty(info.Priority),
				Md5Sum:    hexSum(md5Hash),
				Sha1Sum:   hexSum(sha1Hash),
				Sha256Sum: hexSum(sha256Hash),
			},
		},
	}, nil
}

func changesVersion(info *nfpm.Info) string {
	var version strings.Builder

	if info.Epoch != "" {
		version.WriteString(info.Epoch + ":")
	}

	version.WriteString(info.Version)

	if info.Prerelease != "" {
		version.WriteString("~" + info.Prerelease)
	}

	if info.VersionMetadata != "" {
		version.WriteString("+" + info.VersionMetadata)
	}

	if info.Release != "" {
		version.WriteString("-" + info.Release)
	}

	return version.String()
}

// dashIfEmpty returns the "-" placeholder that the Files field requires for
// an omitted section or priority.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}

	return s
}

func hexSum(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Sum(nil))
}

var errUnsupportedChangesSignFn = errors.New("signing a .changes file with a custom signer requires the dpkg-sig method, which expects a clear-signed result")

func signChanges(data []byte, info *nfpm.Info) ([]byte, error) {
	signConfig := info.Deb.Signature

	if signFn := signConfig.SignFn; signFn != nil {
		if signConfig.Method != "dpkg-sig" {
			return nil, &nfpm.ErrSigningFailure{Err: errUnsupportedChangesSignFn}
		}

		signature, err := signFn(bytes.NewReader(data))
		if err != nil {
			return nil, &nfpm.ErrSigningFailure{Err: err}
		}

		return signature, nil
	}

	signature, err := sign.PGPClearSignWithKeyID(bytes.NewReader(data), signConfig.KeyFile, signConfig.KeyPassphrase, signConfig.KeyID)
	if err != nil {
		return nil, &nfpm.ErrSigningFailure{Err: err}
	}

	return signature, nil
}
