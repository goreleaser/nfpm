// Package rpm implements nfpm.Packager providing .rpm bindings using
// google/rpmpack.
package rpm

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/rpmpack"
	"github.com/pkg/errors"

	"github.com/goreleaser/nfpm"
	"github.com/goreleaser/nfpm/glob"
)

// nolint: gochecknoinits
func init() {
	nfpm.Register("rpm", Default)
}

// Default RPM packager
// nolint: gochecknoglobals
var Default = &RPM{}

// RPM is a RPM packager implementation
type RPM struct{}

// nolint: gochecknoglobals
var archToRPM = map[string]string{
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

// Package implementation.
func (*RPM) Package(info *nfpm.Info, w io.Writer) error {
	var (
		err  error
		meta *rpmpack.RPMMetaData
		rpm  *rpmpack.RPM
	)
	info = ensureValidArch(info)
	if err = nfpm.Validate(info); err != nil {
		return err
	}

	if meta, err = buildRPMMeta(info); err != nil {
		return err
	}
	if rpm, err = rpmpack.NewRPM(*meta); err != nil {
		return err
	}

	addEmptyDirsRPM(info, rpm)
	if err = createFilesInsideRPM(info, rpm); err != nil {
		return err
	}

	if err = addScriptFiles(info, rpm); err != nil {
		return err
	}

	if err = rpm.Write(w); err != nil {
		return err
	}

	return nil
}

func buildRPMMeta(info *nfpm.Info) (*rpmpack.RPMMetaData, error) {
	var (
		err   error
		epoch uint64
		provides,
		depends,
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
		Summary:     strings.Split(info.Description, "\n")[0],
		Description: info.Description,
		Version:     info.Version,
		Release:     releaseFor(info),
		Epoch:       uint32(epoch),
		Arch:        info.Arch,
		OS:          info.Platform,
		Licence:     info.License,
		URL:         info.Homepage,
		Vendor:      info.Vendor,
		Packager:    info.Maintainer,
		Group:       defaultTo(info.RPM.Group, "Development/Tools"),
		Provides:    provides,
		Requires:    depends,
		Obsoletes:   replaces,
		Suggests:    suggests,
		Conflicts:   conflicts,
		Compressor:  info.RPM.Compression,
		BuildTime:   time.Now(),
		BuildHost:   hostname,
	}, nil
}

func releaseFor(info *nfpm.Info) string {
	var release = defaultTo(info.Release, "1")
	if info.Prerelease != "" {
		release = fmt.Sprintf("%s.%s", defaultTo(info.Release, "0.1"), info.Prerelease)
	}
	return release
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

	return nil
}

func addEmptyDirsRPM(info *nfpm.Info, rpm *rpmpack.RPM) {
	for _, dir := range info.EmptyFolders {
		rpm.AddFile(
			rpmpack.RPMFile{
				Name:  dir,
				Mode:  uint(040755),
				MTime: uint32(time.Now().Unix()),
				Owner: "root",
				Group: "root",
			},
		)
	}
}

func createFilesInsideRPM(info *nfpm.Info, rpm *rpmpack.RPM) error {
	copyFunc := func(files map[string]string, config bool) error {
		for srcglob, dstroot := range files {
			globbed, err := glob.Glob(srcglob, dstroot)
			if err != nil {
				return err
			}
			for src, dst := range globbed {
				// when used as a lib, target may not be set.
				// in that case, src will always have the empty sufix, and all
				// files will be ignored.
				if info.Target != "" && strings.HasSuffix(src, info.Target) {
					fmt.Printf("skipping %s because it has the suffix %s", src, info.Target)
					continue
				}
				err := copyToRPM(rpm, src, dst, config)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}
	err := copyFunc(info.Files, false)
	if err != nil {
		return err
	}
	err = copyFunc(info.ConfigFiles, true)
	if err != nil {
		return err
	}
	return nil
}

func copyToRPM(rpm *rpmpack.RPM, src, dst string, config bool) error {
	file, err := os.OpenFile(src, os.O_RDONLY, 0600) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, "could not add file to the archive")
	}
	// don't care if it errs while closing...
	defer file.Close() // nolint: errcheck,gosec
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		// TODO: this should probably return an error
		return nil
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	rpmFile := rpmpack.RPMFile{
		Name:  dst,
		Body:  data,
		Mode:  uint(info.Mode()),
		MTime: uint32(info.ModTime().Unix()),
		Owner: "root",
		Group: "root",
	}

	if config {
		rpmFile.Type = rpmpack.ConfigFile
	}

	rpm.AddFile(rpmFile)

	return nil
}
