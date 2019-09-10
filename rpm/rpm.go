// Package rpm implements nfpm.Packager providing .rpm bindings through rpmbuild.
package rpm

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/google/rpmpack"
	"github.com/goreleaser/nfpm"
	"github.com/goreleaser/nfpm/glob"
	"github.com/pkg/errors"
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

func ensureValidArch(info nfpm.Info) nfpm.Info {
	arch, ok := archToRPM[info.Arch]
	if ok {
		info.Arch = arch
	}
	return info
}

// Package writes a new RPM package to the given writer using the given info
func (*RPM) Package(info nfpm.Info, w io.Writer) error {
	info = ensureValidArch(info)
	err := nfpm.Validate(info)
	if err != nil {
		return err
	}

	vInfo := strings.SplitN(info.Version, "-", 1)
	vInfo = append(vInfo, "")
	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    info.Name,
		Version: vInfo[0],
		Release: vInfo[1],
		Arch:    info.Arch,
	})
	if err != nil {
		return err
	}

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

func addScriptFiles(info nfpm.Info, rpm *rpmpack.RPM) error {
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

func createFilesInsideRPM(info nfpm.Info, rpm *rpmpack.RPM) error {
	for _, files := range []map[string]string{
		info.Files,
		info.ConfigFiles,
	} {
		for srcglob, dstroot := range files {
			globbed, err := glob.Glob(srcglob, dstroot)
			if err != nil {
				return err
			}
			for src, dst := range globbed {
				err := copyToRPM(rpm, src, dst)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copyToRPM(rpm *rpmpack.RPM, src, dst string) error {
	file, err := os.OpenFile(src, os.O_RDONLY, 0600) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, "could not add file to the archive")
	}
	// don't care if it errs while closing...
	defer file.Close() // nolint: errcheck
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

	rpm.AddFile(
		rpmpack.RPMFile{
			Name: dst,
			Body: data,
			Mode: uint(info.Mode()),
		})

	return nil
}
