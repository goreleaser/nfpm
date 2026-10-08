package rpm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
)

// rpmSenseFindRequires marks a dependency as generated, like the ones added by
// the rpmbuild dependency generators.
// https://github.com/rpm-software-management/rpm/blob/master/include/rpm/rpmds.h
const rpmSenseFindRequires = 1 << 14

// The two expressions of rpm's script.req: the interpreter of the #! line, and
// the absolute path of the program run by env.
var (
	shebangInterpreter = regexp.MustCompile(`^#![[:space:]]*(/[^[:space:]]+)`)
	shebangEnvProgram  = regexp.MustCompile(`^#![[:space:]]*[^[:space:]]*/bin/env[[:space:]]+(/[^[:space:]]+)`)
)

// scriptRequires returns the interpreters of the executable scripts of the
// package, read from their #! line.
func scriptRequires(info *nfpm.Info) ([]string, error) {
	if !info.RPM.ScriptRequires {
		return nil, nil
	}

	var interpreters []string
	for _, content := range info.Contents {
		if content.Packager != "" && content.Packager != contentPackager {
			continue
		}
		switch content.Type {
		case files.TypeSymlink, files.TypeDir, files.TypeImplicitDir, files.TypeRPMGhost:
			continue
		case files.TypeRPMDoc, files.TypeRPMReadme:
			// rpmbuild generates no dependencies for documentation.
			continue
		}
		// rpmbuild checks the mode of the file in the build root; the mode
		// in the package is the nfpm equivalent, and the only one that can
		// be executable when building on Windows.
		if content.FileInfo.Mode.Perm()&0o111 == 0 {
			continue
		}

		found, err := shebangRequires(content.Source)
		if err != nil {
			return nil, fmt.Errorf("could not read the interpreter of %s: %w", content.Source, err)
		}
		interpreters = append(interpreters, found...)
	}

	slices.Sort(interpreters)
	return slices.Compact(interpreters), nil
}

func shebangRequires(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	if magic, err := r.Peek(2); err != nil || string(magic) != "#!" {
		return nil, nil
	}
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	var found []string
	for _, re := range []*regexp.Regexp{shebangInterpreter, shebangEnvProgram} {
		if m := re.FindStringSubmatch(line); m != nil {
			found = append(found, m[1])
		}
	}
	return found, nil
}
