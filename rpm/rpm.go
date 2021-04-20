// Package rpm implements nfpm.Packager providing .rpm bindings using
// google/rpmpack.
package rpm

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/rpmpack"
	"github.com/sassoftware/go-rpmutils/cpio"

	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"

	"github.com/goreleaser/chglog"

	"github.com/goreleaser/nfpm/v2"
)

const (
	// https://github.com/rpm-software-management/rpm/blob/master/lib/rpmtag.h#L152
	tagChangelogTime = 1080
	// https://github.com/rpm-software-management/rpm/blob/master/lib/rpmtag.h#L153
	tagChangelogName = 1081
	// https://github.com/rpm-software-management/rpm/blob/master/lib/rpmtag.h#L154
	tagChangelogText = 1082

	changelogNotesTemplate = `
{{- range .Changes }}{{$note := splitList "\n" .Note}}
- {{ first $note }}
{{- range $i,$n := (rest $note) }}{{- if ne (trim $n) ""}}
{{$n}}{{end}}
{{- end}}{{- end}}`
)

const packagerName = "rpm"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// Default RPM packager.
// nolint: gochecknoglobals
var Default = &RPM{}

// RPM is a RPM packager implementation.
type RPM struct{}

// nolint: gochecknoglobals
var archToRPM = map[string]string{
	"all":   "noarch",
	"amd64": "x86_64",
	"386":   "i386",
	"arm64": "aarch64",
}

func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	arch, ok := archToRPM[info.Arch]
	if ok {
		info.Arch = arch
	}
	return info
}

// ConventionalFileName returns a file name according
// to the conventions for RPM packages. See:
// http://ftp.rpm.org/max-rpm/ch-rpm-file-format.html
func (*RPM) ConventionalFileName(info *nfpm.Info) string {
	info = ensureValidArch(info)

	version := formatVersion(info)
	if info.Release != "" {
		version += "-" + info.Release
	}

	// name-version-release.architecture.rpm
	return fmt.Sprintf("%s-%s.%s.rpm", info.Name, version, info.Arch)
}

// Package writes a new RPM package to the given writer using the given info.
func (*RPM) Package(info *nfpm.Info, w io.Writer) (err error) {
	var (
		meta *rpmpack.RPMMetaData
		rpm  *rpmpack.RPM
	)
	info = ensureValidArch(info)
	if err = info.Validate(); err != nil {
		return err
	}

	if meta, err = buildRPMMeta(info); err != nil {
		return err
	}
	if rpm, err = rpmpack.NewRPM(*meta); err != nil {
		return err
	}

	if info.RPM.Signature.KeyFile != "" {
		rpm.SetPGPSigner(sign.PGPSignerWithKeyID(info.RPM.Signature.KeyFile, info.RPM.Signature.KeyPassphrase, info.RPM.Signature.KeyID))
	}

	addEmptyDirsRPM(info, rpm)
	if err = createFilesInsideRPM(info, rpm); err != nil {
		return err
	}

	if err = addScriptFiles(info, rpm); err != nil {
		return err
	}

	if info.Changelog != "" {
		if err = addChangeLog(info, rpm); err != nil {
			return err
		}
	}

	if err = rpm.Write(w); err != nil {
		return err
	}

	return nil
}

func addChangeLog(info *nfpm.Info, rpm *rpmpack.RPM) error {
	changelog, err := info.GetChangeLog()
	if err != nil {
		return fmt.Errorf("reading changelog: %w", err)
	}

	if len(changelog.Entries) == 0 {
		// no nothing because creating empty tags
		// would result in an invalid package
		return nil
	}

	tpl, err := chglog.LoadTemplateData(changelogNotesTemplate)
	if err != nil {
		return fmt.Errorf("parsing RPM changelog template: %w", err)
	}

	changes := make([]string, len(changelog.Entries))
	titles := make([]string, len(changelog.Entries))
	times := make([]uint32, len(changelog.Entries))
	for idx, entry := range changelog.Entries {
		var formattedNotes bytes.Buffer

		err := tpl.Execute(&formattedNotes, entry)
		if err != nil {
			return fmt.Errorf("formatting changlog notes: %w", err)
		}

		changes[idx] = strings.TrimSpace(formattedNotes.String())
		times[idx] = uint32(entry.Date.Unix())
		titles[idx] = fmt.Sprintf("%s - %s", entry.Packager, entry.Semver)
	}

	rpm.AddCustomTag(tagChangelogTime, rpmpack.EntryUint32(times))
	rpm.AddCustomTag(tagChangelogName, rpmpack.EntryStringSlice(titles))
	rpm.AddCustomTag(tagChangelogText, rpmpack.EntryStringSlice(changes))

	return nil
}

//nolint:funlen
func buildRPMMeta(info *nfpm.Info) (*rpmpack.RPMMetaData, error) {
	var (
		err   error
		epoch uint64
		provides,
		depends,
		recommends,
		replaces,
		suggests,
		conflicts rpmpack.Relations
	)
	if epoch, err = strconv.ParseUint(defaultTo(info.Epoch, "0"), 10, 32); err != nil {
		return nil, err
	}
	if provides, err = toRelation(info.Provides); err != nil {
		return nil, err
	}
	if depends, err = toRelation(info.Depends); err != nil {
		return nil, err
	}
	if recommends, err = toRelation(info.Recommends); err != nil {
		return nil, err
	}
	if replaces, err = toRelation(info.Replaces); err != nil {
		return nil, err
	}
	if suggests, err = toRelation(info.Suggests); err != nil {
		return nil, err
	}
	if conflicts, err = toRelation(info.Conflicts); err != nil {
		return nil, err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	return &rpmpack.RPMMetaData{
		Name:        info.Name,
		Summary:     defaultTo(info.RPM.Summary, strings.Split(info.Description, "\n")[0]),
		Description: info.Description,
		Version:     formatVersion(info),
		Release:     defaultTo(info.Release, "1"),
		Epoch:       uint32(epoch),
		Arch:        info.Arch,
		OS:          info.Platform,
		Licence:     info.License,
		URL:         info.Homepage,
		Vendor:      info.Vendor,
		Packager:    info.Maintainer,
		Group:       info.RPM.Group,
		Provides:    provides,
		Recommends:  recommends,
		Requires:    depends,
		Obsoletes:   replaces,
		Suggests:    suggests,
		Conflicts:   conflicts,
		Compressor:  info.RPM.Compression,
		BuildTime:   time.Now(),
		BuildHost:   hostname,
	}, nil
}

func formatVersion(info *nfpm.Info) string {
	version := info.Version

	if info.Prerelease != "" {
		version += "~" + info.Prerelease
	}

	if info.VersionMetadata != "" {
		version += "+" + info.VersionMetadata
	}

	return version
}

func defaultTo(in, def string) string {
	if in == "" {
		return def
	}
	return in
}

func toRelation(items []string) (rpmpack.Relations, error) {
	relations := make(rpmpack.Relations, 0)
	for idx := range items {
		if err := relations.Set(items[idx]); err != nil {
			return nil, err
		}
	}

	return relations, nil
}

func addScriptFiles(info *nfpm.Info, rpm *rpmpack.RPM) error {
	if info.RPM.Scripts.PreTrans != "" {
		data, err := ioutil.ReadFile(info.RPM.Scripts.PreTrans)
		if err != nil {
			return err
		}
		rpm.AddPretrans(string(data))
	}
	if info.Scripts.PreInstall != "" {
		data, err := ioutil.ReadFile(info.Scripts.PreInstall)
		if err != nil {
			return err
		}
		rpm.AddPrein(string(data))
	}

	if info.Scripts.PreRemove != "" {
		data, err := ioutil.ReadFile(info.Scripts.PreRemove)
		if err != nil {
			return err
		}
		rpm.AddPreun(string(data))
	}

	if info.Scripts.PostInstall != "" {
		data, err := ioutil.ReadFile(info.Scripts.PostInstall)
		if err != nil {
			return err
		}
		rpm.AddPostin(string(data))
	}

	if info.Scripts.PostRemove != "" {
		data, err := ioutil.ReadFile(info.Scripts.PostRemove)
		if err != nil {
			return err
		}
		rpm.AddPostun(string(data))
	}

	if info.RPM.Scripts.PostTrans != "" {
		data, err := ioutil.ReadFile(info.RPM.Scripts.PostTrans)
		if err != nil {
			return err
		}
		rpm.AddPosttrans(string(data))
	}

	return nil
}

func addEmptyDirsRPM(info *nfpm.Info, rpm *rpmpack.RPM) {
	for _, dir := range info.EmptyFolders {
		rpm.AddFile(
			rpmpack.RPMFile{
				Name:  dir,
				Mode:  uint(0o40755),
				MTime: uint32(time.Now().Unix()),
				Owner: "root",
				Group: "root",
			},
		)
	}
}

func createFilesInsideRPM(info *nfpm.Info, rpm *rpmpack.RPM) (err error) {
	var symlinks files.Contents
	for _, file := range info.Contents {
		if file.Packager != "" && file.Packager != packagerName {
			continue
		}

		var rpmFileType rpmpack.FileType
		switch file.Type {
		case "config":
			rpmFileType = rpmpack.ConfigFile
		case "config|noreplace":
			rpmFileType = rpmpack.ConfigFile | rpmpack.NoReplaceFile
		case "ghost":
			rpmFileType = rpmpack.GhostFile
			if file.FileInfo.Mode == 0 {
				file.FileInfo.Mode = os.FileMode(0o644)
			}
		case "doc":
			rpmFileType = rpmpack.DocFile
		case "licence", "license":
			rpmFileType = rpmpack.LicenceFile
		case "readme":
			rpmFileType = rpmpack.ReadmeFile
		case "symlink":
			symlinks = append(symlinks, file)
			continue
		default:
			rpmFileType = rpmpack.GenericFile
		}

		if err = copyToRPM(rpm, file, rpmFileType); err != nil {
			return err
		}
	}

	addSymlinksInsideRPM(symlinks, rpm)

	return nil
}

func addSymlinksInsideRPM(symlinks files.Contents, rpm *rpmpack.RPM) {
	for _, file := range symlinks {
		rpm.AddFile(rpmpack.RPMFile{
			Name:  file.Destination,
			Body:  []byte(file.Source),
			Mode:  uint(cpio.S_ISLNK),
			MTime: uint32(file.FileInfo.MTime.Unix()),
			Owner: file.FileInfo.Owner,
			Group: file.FileInfo.Group,
		})
	}
}

func copyToRPM(rpm *rpmpack.RPM, file *files.Content, fileType rpmpack.FileType) (err error) {
	data, err := ioutil.ReadFile(file.Source)
	if err != nil && file.Type != "ghost" {
		return err
	}

	rpm.AddFile(rpmpack.RPMFile{
		Name:  file.Destination,
		Body:  data,
		Mode:  uint(file.FileInfo.Mode),
		MTime: uint32(file.FileInfo.MTime.Unix()),
		Owner: file.FileInfo.Owner,
		Group: file.FileInfo.Group,
		Type:  fileType,
	})

	return nil
}
