package acceptance

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v1"

	"github.com/goreleaser/nfpm"
	// shut up
	_ "github.com/goreleaser/nfpm/deb"
	_ "github.com/goreleaser/nfpm/rpm"
)

func accept(t *testing.T, name, conf, format, dockerfile string) {
	var config = filepath.Join("./testdata", conf)
	tmp, err := filepath.Abs("./testdata/tmp")
	require.NoError(t, err)
	var packageName = name + "." + format
	var target = filepath.Join(tmp, packageName)

	require.NoError(t, os.MkdirAll(tmp, 0700))

	bts, err := ioutil.ReadFile(config)
	require.NoError(t, err)

	var info nfpm.Info
	err = yaml.Unmarshal(bts, &info)
	require.NoError(t, err)
	pkg, err := nfpm.Get(format)
	require.NoError(t, err)

	f, err := os.Create(target)
	require.NoError(t, err)
	require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))
	bts, _ = exec.Command("pwd").CombinedOutput()
	t.Log(string(bts))
	cmd := exec.Command(
		"docker",
		"build",
		"-f",
		filepath.Join("./testdata", dockerfile),
		"--build-arg",
		"package="+filepath.Join("tmp", packageName),
		"./testdata",
	)
	t.Log("will exec:", cmd.Args)
	bts, err = cmd.CombinedOutput()
	t.Log("output:", string(bts))
	require.NoError(t, err)
}
