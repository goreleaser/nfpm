package acceptance

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v1"

	_ "github.com/goreleaser/nfpm/deb"
	_ "github.com/goreleaser/nfpm/rpm"
)

func TestSimpleDeb(t *testing.T) {
	var config = "./testdata/simple_deb.yaml"
	var format = "deb"
	tmp, err := filepath.Abs("./testdata/tmp/simple_deb")
	require.NoError(t, err)
	t.Log("tmpdir:", tmp)
	var target = filepath.Join(tmp, "foo."+format)

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
	cmd := exec.Command("docker", "build", "-f", "./testdata/simple_deb.dockerfile", "./testdata")
	t.Log(cmd.Args)
	bts, err = cmd.CombinedOutput()
	assert.NoError(t, err)
	t.Log(string(bts))
}
