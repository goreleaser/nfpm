package rpm

import (
	"os"

	"github.com/goreleaser/nfpm/v2"
	"go.digitalxero.dev/rpm"
)

// scriptBodies holds the resolved (file-read) bodies of every lifecycle script.
type scriptBodies struct {
	preTrans  string
	preIn     string
	preUn     string
	postIn    string
	postUn    string
	postTrans string
	verify    string
}

// readScripts reads every configured script file into memory once, so both the
// binary builder and the source-package spec generator can consume them.
func readScripts(info *nfpm.Info) (scriptBodies, error) {
	read := func(path string) (string, error) {
		if path == "" {
			return "", nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var (
		s   scriptBodies
		err error
	)
	if s.preTrans, err = read(info.RPM.Scripts.PreTrans); err != nil {
		return s, err
	}
	if s.preIn, err = read(info.Scripts.PreInstall); err != nil {
		return s, err
	}
	if s.preUn, err = read(info.Scripts.PreRemove); err != nil {
		return s, err
	}
	if s.postIn, err = read(info.Scripts.PostInstall); err != nil {
		return s, err
	}
	if s.postUn, err = read(info.Scripts.PostRemove); err != nil {
		return s, err
	}
	if s.postTrans, err = read(info.RPM.Scripts.PostTrans); err != nil {
		return s, err
	}
	if s.verify, err = read(info.RPM.Scripts.Verify); err != nil {
		return s, err
	}
	return s, nil
}

// applyScripts attaches the configured lifecycle scripts to the package builder.
func applyScripts(b rpm.PackageBuilder, info *nfpm.Info) error {
	s, err := readScripts(info)
	if err != nil {
		return err
	}

	if s.preTrans != "" {
		b.Pretrans().WithScript(s.preTrans).Done()
	}
	if s.preIn != "" {
		b.Prein().WithScript(s.preIn).Done()
	}
	if s.preUn != "" {
		b.Preun().WithScript(s.preUn).Done()
	}
	if s.postIn != "" {
		b.Postin().WithScript(s.postIn).Done()
	}
	if s.postUn != "" {
		b.Postun().WithScript(s.postUn).Done()
	}
	if s.postTrans != "" {
		b.Posttrans().WithScript(s.postTrans).Done()
	}
	if s.verify != "" {
		b.VerifyScript().WithScript(s.verify).Done()
	}
	return nil
}
