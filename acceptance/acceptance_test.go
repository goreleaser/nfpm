package acceptance

import (
	"fmt"
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

var formats = []string{"deb", "rpm"}

func TestSimple(t *testing.T) {
	for _, format := range formats {
		t.Run("amd64", func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:       fmt.Sprintf("simple_%s", format),
				Conf:       "simple.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.dockerfile", format),
			})
		})
		t.Run("i386", func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:       fmt.Sprintf("simple_%s_386", format),
				Conf:       "simple.386.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.386.dockerfile", format),
			})
		})
	}
}

func TestComplex(t *testing.T) {
	for _, format := range formats {
		t.Run("amd64", func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:       fmt.Sprintf("complex_%s", format),
				Conf:       "complex.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.complex.dockerfile", format),
			})
		})
		t.Run("i386", func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:       fmt.Sprintf("complex_%s_386", format),
				Conf:       "complex.386.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.386.complex.dockerfile", format),
			})
		})
	}
}

func TestComplexOverridesDeb(t *testing.T) {
	for _, format := range formats {
		t.Run("amd64", func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:       fmt.Sprintf("overrides_%s", format),
				Conf:       "overrides.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.overrides.dockerfile", format),
			})
		})
	}
}

func TestMinDeb(t *testing.T) {
	for _, format := range formats {
		t.Run("amd64", func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:       fmt.Sprintf("min_%s", format),
				Conf:       "min.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.min.dockerfile", format),
			})
		})
	}
}

type acceptParms struct {
	Name       string
	Conf       string
	Format     string
	Dockerfile string
}

func accept(t *testing.T, params acceptParms) {
	var configFile = filepath.Join("./testdata", params.Conf)
	tmp, err := filepath.Abs("./testdata/tmp")
	require.NoError(t, err)
	var packageName = params.Name + "." + params.Format
	var target = filepath.Join(tmp, packageName)

	require.NoError(t, os.MkdirAll(tmp, 0700))

	config, err := nfpm.ParseFile(configFile)
	require.NoError(t, err)

	info, err := config.Get(params.Format)
	require.NoError(t, err)
	require.NoError(t, nfpm.Validate(info))

	pkg, err := nfpm.Get(params.Format)
	require.NoError(t, err)

	f, err := os.Create(target)
	require.NoError(t, err)
	require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))
	//nolint:gosec
	cmd := exec.Command(
		"docker", "build", "--rm", "--force-rm",
		"-f", params.Dockerfile,
		"--build-arg", "package="+filepath.Join("tmp", packageName),
		".",
	)
	cmd.Dir = "./testdata"
	t.Log("will exec:", cmd.Args)
	bts, err := cmd.CombinedOutput()
	t.Log("output:", string(bts))
	require.NoError(t, err)
}
