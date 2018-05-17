package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm"
	// shut up
	_ "github.com/goreleaser/nfpm/deb"
	_ "github.com/goreleaser/nfpm/rpm"
)

func accept(t *testing.T, name, conf, format, dockerfile string) {
	var configFile = filepath.Join("./testdata", conf)
	tmp, err := filepath.Abs("./testdata/tmp")
	require.NoError(t, err)
	var packageName = name + "." + format
	var target = filepath.Join(tmp, packageName)

	require.NoError(t, os.MkdirAll(tmp, 0700))

	config, err := nfpm.ParseFile(configFile)
	require.NoError(t, err)

	info, err := config.Get(format)
	require.NoError(t, err)
	require.NoError(t, nfpm.Validate(info))

	pkg, err := nfpm.Get(format)
	require.NoError(t, err)

	f, err := os.Create(target)
	require.NoError(t, err)
	require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))
	bts, _ := exec.Command("pwd").CombinedOutput()
	t.Log(string(bts))
	cmd := exec.Command(
		"docker", "build", "--rm", "--force-rm",
		"-f", dockerfile,
		"--build-arg", "package="+filepath.Join("tmp", packageName),
		".",
	)
	cmd.Dir = "./testdata"
	t.Log("will exec:", cmd.Args)
	bts, err = cmd.CombinedOutput()
	t.Log("output:", string(bts))
	require.NoError(t, err)
}
