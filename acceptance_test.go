// +build acceptance

package nfpm_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm/v2"
	_ "github.com/goreleaser/nfpm/v2/apk"
	_ "github.com/goreleaser/nfpm/v2/deb"
	_ "github.com/goreleaser/nfpm/v2/rpm"
)

// nolint: gochecknoglobals
var formats = []string{"deb", "rpm", "apk"}

func TestSimple(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("amd64-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("simple_%s", format),
				Conf:       "simple.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.dockerfile", format),
			})
		})
		t.Run(fmt.Sprintf("i386-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("simple_%s_386", format),
				Conf:       "simple.386.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.386.dockerfile", format),
			})
		})
		t.Run(fmt.Sprintf("ppc64le-%s", format), func(t *testing.T) {
			t.Skip("for some reason travis fails to run those")
			accept(t, acceptParms{
				Name:       fmt.Sprintf("simple_%s_ppc64le", format),
				Conf:       "simple.ppc64le.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.ppc64le.dockerfile", format),
			})
		})
		t.Run(fmt.Sprintf("arm64-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("simple_%s_arm64", format),
				Conf:       "simple.arm64.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.arm64.dockerfile", format),
			})
		})
	}
}

func TestComplex(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("amd64-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("complex_%s", format),
				Conf:       "complex.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.complex.dockerfile", format),
			})
		})
		t.Run(fmt.Sprintf("i386-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("complex_%s_386", format),
				Conf:       "complex.386.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.386.complex.dockerfile", format),
			})
		})
	}
}

func TestConfigNoReplace(t *testing.T) {
	var target = "./testdata/acceptance/tmp/noreplace_old_rpm.rpm"
	require.NoError(t, os.MkdirAll("./testdata/acceptance/tmp", 0700))

	config, err := nfpm.ParseFile("./testdata/acceptance/config-noreplace-old.yaml")
	require.NoError(t, err)

	info, err := config.Get("rpm")
	require.NoError(t, err)
	require.NoError(t, nfpm.Validate(info))

	pkg, err := nfpm.Get("rpm")
	require.NoError(t, err)

	f, err := os.Create(target)
	require.NoError(t, err)
	info.Target = target
	require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))

	t.Run("rpm", func(t *testing.T) {
		accept(t, acceptParms{
			Name:       "noreplace_rpm",
			Conf:       "config-noreplace.yaml",
			Format:     "rpm",
			Dockerfile: "rpm.config-noreplace.dockerfile",
		})
	})
}

func TestEnvVarVersion(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("amd64-%s", format), func(t *testing.T) {
			os.Setenv("SEMVER", "v1.0.0-0.1.b1+git.abcdefgh")
			accept(t, acceptParms{
				Name:       fmt.Sprintf("env-var-version_%s", format),
				Conf:       "env-var-version.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.env-var-version.dockerfile", format),
			})
		})
	}
}

func TestComplexOverrides(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("amd64-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("overrides_%s", format),
				Conf:       "overrides.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.overrides.dockerfile", format),
			})
		})
	}
}

func TestMin(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("amd64-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("min_%s", format),
				Conf:       "min.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.min.dockerfile", format),
			})
		})
	}
}

func TestMeta(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("amd64-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("meta_%s", format),
				Conf:       "meta.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.meta.dockerfile", format),
			})
		})
	}
}

func TestRPMCompression(t *testing.T) {
	compressFormats := []string{"gzip", "xz", "lzma"}
	for _, format := range compressFormats {
		format := format
		t.Run(format, func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("%s_compression_rpm", format),
				Conf:       fmt.Sprintf("%s.compression.yaml", format),
				Format:     "rpm",
				Dockerfile: fmt.Sprintf("%s.rpm.compression.dockerfile", format),
			})
		})
	}
}

func TestRPMRelease(t *testing.T) {
	accept(t, acceptParms{
		Name:       "release_rpm",
		Conf:       "release.rpm.yaml",
		Format:     "rpm",
		Dockerfile: "release.rpm.dockerfile",
	})
}

func TestDebRules(t *testing.T) {
	accept(t, acceptParms{
		Name:       "rules.deb",
		Conf:       "rules.deb.yaml",
		Format:     "deb",
		Dockerfile: "rules.deb.dockerfile",
	})
}

func TestChangelog(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("changelog-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("changelog_%s", format),
				Conf:       "withchangelog.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.changelog.dockerfile", format),
			})
		})
	}
}

func TestDebTriggers(t *testing.T) {
	t.Run("triggers-deb", func(t *testing.T) {
		accept(t, acceptParms{
			Name:       "triggers-deb",
			Conf:       "triggers.yaml",
			Format:     "deb",
			Dockerfile: "deb.triggers.dockerfile",
		})
	})
}

func TestSymlink(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run(fmt.Sprintf("symlink-%s", format), func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("symlink_%s", format),
				Conf:       "symlink.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.symlink.dockerfile", format),
			})
		})
	}
}

func TestDebBreaks(t *testing.T) {
	t.Run("breaks-deb", func(t *testing.T) {
		accept(t, acceptParms{
			Name:       "breaks-deb",
			Conf:       "breaks.yaml",
			Format:     "deb",
			Dockerfile: "deb.breaks.dockerfile",
		})
	})
}

func TestSignatures(t *testing.T) {
	for _, format := range formats {
		format := format
		t.Run("signed", func(t *testing.T) {
			accept(t, acceptParms{
				Name:       fmt.Sprintf("signed_%s", format),
				Conf:       "signed.yaml",
				Format:     format,
				Dockerfile: fmt.Sprintf("%s.signed.dockerfile", format),
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

type testWriter struct {
	t *testing.T
}

func (t testWriter) Write(p []byte) (n int, err error) {
	t.t.Log(string(p))
	return len(p), nil
}

func accept(t *testing.T, params acceptParms) {
	var configFile = filepath.Join("./testdata/acceptance/", params.Conf)
	tmp, err := filepath.Abs("./testdata/acceptance/tmp")
	require.NoError(t, err)
	var packageName = params.Name + "." + params.Format
	var target = filepath.Join(tmp, packageName)
	t.Log("package: " + target)

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
	info.Target = target
	require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))
	//nolint:gosec
	cmd := exec.Command(
		"docker", "build", "--rm", "--force-rm",
		"-f", params.Dockerfile,
		"--build-arg", "package="+filepath.Join("tmp", packageName),
		".",
	)
	cmd.Dir = "./testdata/acceptance"
	cmd.Stderr = testWriter{t}
	cmd.Stdout = cmd.Stderr

	t.Log("will exec:", cmd.Args)
	require.NoError(t, cmd.Run())
}
