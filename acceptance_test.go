//go:build acceptance
// +build acceptance

package nfpm_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	_ "github.com/goreleaser/nfpm/v2/apk"
	_ "github.com/goreleaser/nfpm/v2/arch"
	_ "github.com/goreleaser/nfpm/v2/deb"
	_ "github.com/goreleaser/nfpm/v2/rpm"
	"github.com/stretchr/testify/require"
)

// nolint: gochecknoglobals
var formatArchs = map[string][]string{
	"apk":       {"amd64", "arm64", "386", "ppc64le", "armv6", "armv7", "s390x"},
	"deb":       {"amd64", "arm64", "ppc64le", "armv7", "s390x"},
	"rpm":       {"amd64", "arm64", "ppc64le"},
	"archlinux": {"amd64"},
}

func TestCore(t *testing.T) {
	t.Parallel()
	testNames := []string{
		"min",
		"simple",
		"no-glob",
		"complex",
		"env-var-version",
		"overrides",
		"meta",
		"withchangelog",
		"symlink",
		"signed",
	}
	for _, name := range testNames {
		for format, architecture := range formatArchs {
			for _, arch := range architecture {
				func(t *testing.T, testName, testFormat, testArch string) {
					t.Run(fmt.Sprintf("%s/%s/%s", testFormat, testArch, testName), func(t *testing.T) {
						t.Parallel()
						if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
							t.Skip("ppc64le arch not supported in pipeline")
						}
						accept(t, acceptParms{
							Name:   fmt.Sprintf("%s_%s", testName, testArch),
							Conf:   fmt.Sprintf("core.%s.yaml", testName),
							Format: testFormat,
							Docker: dockerParams{
								File:   fmt.Sprintf("%s.dockerfile", testFormat),
								Target: testName,
								Arch:   testArch,
							},
						})
					})
				}(t, name, format, arch)
			}
		}
	}
}

func TestUpgrade(t *testing.T) {
	t.Parallel()
	testNames := []string{
		"upgrade",
	}
	for _, name := range testNames {
		for format, architecture := range formatArchs {
			for _, arch := range architecture {
				func(t *testing.T, testName, testFormat, testArch string) {
					t.Run(fmt.Sprintf("%s/%s/%s", testFormat, testArch, testName), func(t *testing.T) {
						t.Parallel()
						if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
							t.Skip("ppc64le arch not supported in pipeline")
						}

						arch := strings.ReplaceAll(testArch, "armv", "arm/")
						oldpkg := fmt.Sprintf("tmp/%s_%s.v1.%s", testName, testArch, testFormat)
						target := fmt.Sprintf("./testdata/acceptance/%s", oldpkg)
						require.NoError(t, os.MkdirAll("./testdata/acceptance/tmp", 0o700))

						config, err := nfpm.ParseFileWithEnvMapping(fmt.Sprintf("./testdata/acceptance/%s.v1.yaml", testName),
							func(s string) string {
								switch s {
								case "BUILD_ARCH":
									return strings.ReplaceAll(arch, "/", "")
								case "SEMVER":
									return "v1.0.0-0.1.b1+git.abcdefgh"
								default:
									return os.Getenv(s)
								}
							},
						)
						require.NoError(t, err)

						info, err := config.Get(testFormat)
						require.NoError(t, err)
						require.NoError(t, nfpm.Validate(info))

						pkg, err := nfpm.Get(testFormat)
						require.NoError(t, err)

						f, err := os.Create(target)
						require.NoError(t, err)
						defer f.Close()
						info.Target = target
						require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))

						accept(t, acceptParms{
							Name:   fmt.Sprintf("%s_%s.v2", testName, testArch),
							Conf:   fmt.Sprintf("%s.v2.yaml", testName),
							Format: testFormat,
							Docker: dockerParams{
								File:      fmt.Sprintf("%s.dockerfile", testFormat),
								Target:    testName,
								Arch:      testArch,
								BuildArgs: []string{fmt.Sprintf("oldpackage=%s", oldpkg)},
							},
						})
					})
				}(t, name, format, arch)
			}
		}
	}
}

func TestRPMCompression(t *testing.T) {
	t.Parallel()
	format := "rpm"
	compressFormats := []string{"gzip", "xz", "lzma", "zstd"}
	for _, arch := range formatArchs[format] {
		for _, compFormat := range compressFormats {
			func(t *testing.T, testCompFormat, testArch string) {
				t.Run(fmt.Sprintf("%s/%s/%s", format, testArch, testCompFormat), func(t *testing.T) {
					t.Parallel()
					if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
						t.Skip("ppc64le arch not supported in pipeline")
					}
					accept(t, acceptParms{
						Name:   fmt.Sprintf("%s_compression_%s", testCompFormat, testArch),
						Conf:   fmt.Sprintf("rpm.%s.compression.yaml", testCompFormat),
						Format: format,
						Docker: dockerParams{
							File:      fmt.Sprintf("%s.dockerfile", format),
							Target:    "compression",
							Arch:      testArch,
							BuildArgs: []string{fmt.Sprintf("compression=%s", testCompFormat)},
						},
					})
				})
			}(t, compFormat, arch)
		}
	}
}

func TestDebCompression(t *testing.T) {
	t.Parallel()
	format := "deb"
	compressFormats := []string{"gzip", "xz", "zstd", "none"}
	for _, arch := range formatArchs[format] {
		for _, compFormat := range compressFormats {
			func(t *testing.T, testCompFormat, testArch string) {
				t.Run(fmt.Sprintf("%s/%s/%s", format, testArch, testCompFormat), func(t *testing.T) {
					t.Parallel()
					if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
						t.Skip("ppc64le arch not supported in pipeline")
					}

					target := "compression"
					if testCompFormat == "zstd" {
						// we can remove this exception as soon as the debian image supports zstd
						target = "zstdcompression"
					}

					accept(t, acceptParms{
						Name:   fmt.Sprintf("%s_compression_%s", testCompFormat, testArch),
						Conf:   fmt.Sprintf("deb.%s.compression.yaml", testCompFormat),
						Format: format,
						Docker: dockerParams{
							File:   fmt.Sprintf("%s.dockerfile", format),
							Target: target,
							Arch:   testArch,
						},
					})
				})
			}(t, compFormat, arch)
		}
	}
}

func TestRPMSpecific(t *testing.T) {
	t.Parallel()
	format := "rpm"
	testNames := []string{
		"release",
		"directories",
	}
	for _, name := range testNames {
		for _, arch := range formatArchs[format] {
			func(t *testing.T, testName, testArch string) {
				t.Run(fmt.Sprintf("%s/%s/%s", format, testArch, testName), func(t *testing.T) {
					t.Parallel()
					if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
						t.Skip("ppc64le arch not supported in pipeline")
					}
					accept(t, acceptParms{
						Name:   fmt.Sprintf("%s_%s", testName, testArch),
						Conf:   fmt.Sprintf("%s.%s.yaml", format, testName),
						Format: format,
						Docker: dockerParams{
							File:   fmt.Sprintf("%s.dockerfile", format),
							Target: testName,
							Arch:   testArch,
						},
					})
				})
			}(t, name, arch)
		}
	}
}

func TestDebSpecific(t *testing.T) {
	t.Parallel()
	format := "deb"
	testNames := []string{
		"rules",
		"triggers",
		"breaks",
	}
	for _, name := range testNames {
		for _, arch := range formatArchs[format] {
			func(t *testing.T, testName, testArch string) {
				t.Run(fmt.Sprintf("%s/%s/%s", format, testArch, testName), func(t *testing.T) {
					t.Parallel()
					if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
						t.Skip("ppc64le arch not supported in pipeline")
					}
					accept(t, acceptParms{
						Name:   fmt.Sprintf("%s_%s", testName, testArch),
						Conf:   fmt.Sprintf("%s.%s.yaml", format, testName),
						Format: format,
						Docker: dockerParams{
							File:   fmt.Sprintf("%s.dockerfile", format),
							Target: testName,
							Arch:   testArch,
						},
					})
				})
			}(t, name, arch)
		}
	}
}

func TestDebSign(t *testing.T) {
	t.Parallel()
	for _, arch := range formatArchs["deb"] {
		for _, sigtype := range []string{"dpkg-sig", "debsign"} {
			func(t *testing.T, testSigtype, testArch string) {
				t.Run(fmt.Sprintf("%s/%s", testArch, testSigtype), func(t *testing.T) {
					target := "signed"
					if testSigtype == "dpkg-sig" {
						target = "dpkg-signed"
					}
					t.Parallel()
					if testArch == "ppc64le" && os.Getenv("NO_TEST_PPC64LE") == "true" {
						t.Skip("ppc64le arch not supported in pipeline")
					}
					accept(t, acceptParms{
						Name:   fmt.Sprintf("%s_sign_%s", testSigtype, testArch),
						Conf:   fmt.Sprintf("deb.%s.sign.yaml", testSigtype),
						Format: "deb",
						Docker: dockerParams{
							File:   "deb.dockerfile",
							Target: target,
							Arch:   testArch,
						},
					})
				})
			}(t, sigtype, arch)
		}
	}
}

type acceptParms struct {
	Name   string
	Conf   string
	Format string
	Docker dockerParams
}

type dockerParams struct {
	File      string
	Target    string
	Arch      string
	BuildArgs []string
}

func accept(t *testing.T, params acceptParms) {
	t.Helper()

	arch := strings.ReplaceAll(params.Docker.Arch, "armv", "arm/")
	configFile := filepath.Join("./testdata/acceptance/", params.Conf)
	tmp, err := filepath.Abs("./testdata/acceptance/tmp")
	require.NoError(t, err)
	packageName := params.Name + "." + params.Format
	target := filepath.Join(tmp, packageName)
	require.NoError(t, os.MkdirAll(tmp, 0o700))

	envFunc := func(s string) string {
		switch s {
		case "BUILD_ARCH":
			return strings.ReplaceAll(arch, "/", "")
		case "SEMVER":
			return "v1.0.0-0.1.b1+git.abcdefgh"
		default:
			return os.Getenv(s)
		}
	}
	config, err := nfpm.ParseFileWithEnvMapping(configFile, envFunc)
	require.NoError(t, err)

	info, err := config.Get(params.Format)
	require.NoError(t, err)
	require.NoError(t, nfpm.Validate(info))

	pkg, err := nfpm.Get(params.Format)
	require.NoError(t, err)

	cmdArgs := []string{
		"build", "--rm", "--force-rm",
		"--platform", fmt.Sprintf("linux/%s", arch),
		"-f", params.Docker.File,
		"--target", params.Docker.Target,
		"--build-arg", "package=" + filepath.Join("tmp", packageName),
	}
	for _, arg := range params.Docker.BuildArgs {
		cmdArgs = append(cmdArgs, "--build-arg", arg)
	}
	cmdArgs = append(cmdArgs, ".")

	f, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o764)
	require.NoError(t, err)
	info.Target = target
	require.NoError(t, pkg.Package(nfpm.WithDefaults(info), f))
	//nolint:gosec
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Dir = "./testdata/acceptance"
	bts, err := cmd.CombinedOutput()
	require.NoError(
		t,
		err,
		"failed: %v; env BUILD_ARCH: %s; package: %s; output: %s",
		cmd.Args,
		envFunc("BUILD_ARCH"),
		target,
		string(bts),
	)
}
