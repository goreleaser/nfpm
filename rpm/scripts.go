package rpm

import (
	"os"
	"strings"

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

const defaultInterpreter = "/bin/sh"

type scriptlet struct {
	section     string
	body        string
	interpreter string
	sense       rpm.Sense
	builder     func(rpm.PackageBuilder) rpm.ScriptletBuilder
}

// scriptlets pairs every script body with its interpreter, in the order of the
// generated spec.
func (s scriptBodies) scriptlets(in nfpm.RPMInterpreters) []scriptlet {
	return []scriptlet{
		{"pre", s.preIn, defaultTo(in.PreInstall, defaultInterpreter), rpm.SenseScriptPre, rpm.PackageBuilder.Prein},
		{"post", s.postIn, defaultTo(in.PostInstall, defaultInterpreter), rpm.SenseScriptPost, rpm.PackageBuilder.Postin},
		{"preun", s.preUn, defaultTo(in.PreRemove, defaultInterpreter), rpm.SenseScriptPreUn, rpm.PackageBuilder.Preun},
		{"postun", s.postUn, defaultTo(in.PostRemove, defaultInterpreter), rpm.SenseScriptPostUn, rpm.PackageBuilder.Postun},
		{"pretrans", s.preTrans, defaultTo(in.PreTrans, defaultInterpreter), rpm.SensePreTrans, rpm.PackageBuilder.Pretrans},
		{"posttrans", s.postTrans, defaultTo(in.PostTrans, defaultInterpreter), rpm.SensePostTrans, rpm.PackageBuilder.Posttrans},
		{"verifyscript", s.verify, defaultTo(in.Verify, defaultInterpreter), rpm.SenseScriptVerify, rpm.PackageBuilder.VerifyScript},
	}
}

// applyScripts attaches the configured lifecycle scripts to the package builder.
func applyScripts(b rpm.PackageBuilder, info *nfpm.Info) error {
	s, err := readScripts(info)
	if err != nil {
		return err
	}

	for _, sc := range s.scriptlets(info.RPM.Interpreters) {
		if sc.body == "" {
			continue
		}
		sc.builder(b).WithScript(sc.body).WithInterpreter(sc.interpreter).Done()

		// Require the interpreter so that it is installed before the scriptlet
		// runs.
		if strings.HasPrefix(sc.interpreter, "/") {
			b.Requires().With(sc.interpreter, "", sc.sense|rpm.SenseInterp).Done()
		}
	}
	return nil
}
