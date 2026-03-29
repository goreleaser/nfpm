//go:build acceptance

package nfpm_test

import (
	"archive/zip"
	"bytes"
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
	_ "github.com/goreleaser/nfpm/v2/ipk"
	_ "github.com/goreleaser/nfpm/v2/msix"
	_ "github.com/goreleaser/nfpm/v2/rpm"
	_ "github.com/goreleaser/nfpm/v2/xbps"
	"github.com/stretchr/testify/require"
)

// nolint: gochecknoglobals
var formatArchs = map[string][]string{
	"apk":       {"amd64", "arm64", "386", "ppc64le", "armv6", "armv7", "s390x"},
	"deb":       {"amd64", "arm64", "ppc64le", "armv7", "s390x"},
	"ipk":       {"x86_64", "aarch64_generic"},
	"rpm":       {"amd64", "arm64", "ppc64le"},
	"archlinux": {"amd64"},
	"xbps":      {"amd64"},
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
						if testFormat == "xbps" {
							t.Skip("covered by TestXBPSAcceptance")
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
						if testFormat == "xbps" {
							t.Skip("covered by TestXBPSAcceptance")
						}

						acceptWithOldPackage(t, testName, testFormat, testArch,
							fmt.Sprintf("%s.v1.yaml", testName),
							fmt.Sprintf("%s.v2.yaml", testName),
							dockerParams{
								File:   fmt.Sprintf("%s.dockerfile", testFormat),
								Target: testName,
								Arch:   testArch,
							},
						)
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

					accept(t, acceptParms{
						Name:   fmt.Sprintf("%s_compression_%s", testCompFormat, testArch),
						Conf:   fmt.Sprintf("deb.%s.compression.yaml", testCompFormat),
						Format: format,
						Docker: dockerParams{
							File:   fmt.Sprintf("%s.dockerfile", format),
							Target: "compression",
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
		"verify",
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
		"predepends",
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

func TestIPKSpecific(t *testing.T) {
	t.Parallel()
	format := "ipk"
	testNames := []string{
		"alternatives",
		"conflicts",
		"predepends",
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

func TestRPMSign(t *testing.T) {
	for _, os := range []string{
		"centos9",
		"centos10",
		"fedora40",
		"fedora41",
	} {
		os := os
		t.Run(fmt.Sprintf("rpm/amd64/sign/%s", os), func(t *testing.T) {
			accept(t, acceptParms{
				Name:   fmt.Sprintf("sign_%s_amd64", os),
				Conf:   "core.signed.yaml",
				Format: "rpm",
				Docker: dockerParams{
					File:   fmt.Sprintf("rpm_%s.dockerfile", os),
					Target: "signed",
					Arch:   "amd64",
				},
			})
		})
	}
}

func TestDebSign(t *testing.T) {
	t.Parallel()
	for _, arch := range formatArchs["deb"] {
		for _, sigtype := range []string{"dpkg-sig", "debsign"} {
			func(t *testing.T, testSigtype, testArch string) {
				t.Run(fmt.Sprintf("deb/%s/%s", testArch, testSigtype), func(t *testing.T) {
					t.Parallel()
					target := "signed"
					if testSigtype == "dpkg-sig" {
						target = "dpkg-signed"
					}
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
	NoCache   bool
}

func acceptPackageTarget(t *testing.T, stageName, packageName string) (string, string) {
	t.Helper()

	relDir := filepath.Join("tmp", stageName)
	absDir := filepath.Join("./testdata/acceptance", relDir)
	require.NoError(t, os.MkdirAll(absDir, 0o700))

	relTarget := filepath.ToSlash(filepath.Join(relDir, packageName))
	absTarget := filepath.Join("./testdata/acceptance", relTarget)
	return relTarget, absTarget
}

func acceptWithOldPackage(t *testing.T, name, format, arch, oldConf, newConf string, docker dockerParams) {
	t.Helper()

	mappedArch := strings.ReplaceAll(arch, "armv", "arm/")
	oldPackageName := fmt.Sprintf("%s_%s.v1.%s", name, arch, format)
	require.NoError(t, os.MkdirAll("./testdata/acceptance/tmp", 0o700))

	config, err := nfpm.ParseFileWithEnvMapping(filepath.Join("./testdata/acceptance", oldConf),
		func(s string) string {
			switch s {
			case "BUILD_ARCH":
				return strings.ReplaceAll(mappedArch, "/", "")
			case "SEMVER":
				return "v1.0.0-0.1.b1+git.abcdefgh"
			default:
				return os.Getenv(s)
			}
		},
	)
	require.NoError(t, err)

	info, err := config.Get(format)
	require.NoError(t, err)
	require.NoError(t, nfpm.Validate(info))

	pkg, err := nfpm.Get(format)
	require.NoError(t, err)

	preparedInfo := nfpm.WithDefaults(info)
	if format == "xbps" {
		oldPackageName = pkg.ConventionalFileName(preparedInfo)
	}
	oldpkg, target := acceptPackageTarget(t, fmt.Sprintf("%s_%s.v1", name, arch), oldPackageName)

	f, err := os.Create(target)
	require.NoError(t, err)
	preparedInfo.Target = target
	require.NoError(t, pkg.Package(preparedInfo, f))
	require.NoError(t, f.Close())

	accept(t, acceptParms{
		Name:   fmt.Sprintf("%s_%s.v2", name, arch),
		Conf:   newConf,
		Format: format,
		Docker: dockerParams{
			File:      docker.File,
			Target:    docker.Target,
			Arch:      docker.Arch,
			BuildArgs: append(append([]string{}, docker.BuildArgs...), fmt.Sprintf("oldpackage=%s", oldpkg)),
		},
	})
}

func accept(t *testing.T, params acceptParms) {
	t.Helper()

	arch := strings.ReplaceAll(params.Docker.Arch, "armv", "arm/")
	configFile := filepath.Join("./testdata/acceptance/", params.Conf)
	packageName := params.Name + "." + params.Format

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

	preparedInfo := nfpm.WithDefaults(info)
	if params.Format == "xbps" {
		packageName = pkg.ConventionalFileName(preparedInfo)
	}
	relTarget, target := acceptPackageTarget(t, params.Name, packageName)

	cmdArgs := []string{
		"build", "--rm", "--force-rm",
		"--platform", fmt.Sprintf("linux/%s", arch),
	}
	if params.Docker.NoCache {
		cmdArgs = append(cmdArgs, "--no-cache")
	}
	cmdArgs = append(cmdArgs,
		"-f", params.Docker.File,
		"--target", params.Docker.Target,
		"--build-arg", "package="+relTarget,
	)
	for _, arg := range params.Docker.BuildArgs {
		cmdArgs = append(cmdArgs, "--build-arg", arg)
	}
	cmdArgs = append(cmdArgs, ".")

	f, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o764)
	require.NoError(t, err)
	preparedInfo.Target = target
	require.NoError(t, pkg.Package(preparedInfo, f))
	require.NoError(t, f.Close())
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

func TestXBPSAcceptance(t *testing.T) {
	t.Parallel()
	for _, arch := range formatArchs["xbps"] {
		arch := arch
		t.Run(fmt.Sprintf("xbps/%s/lifecycle", arch), func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:   fmt.Sprintf("lifecycle_%s", arch),
				Conf:   "xbps.lifecycle.yaml",
				Format: "xbps",
				Docker: dockerParams{
					File:    "xbps.dockerfile",
					Target:  "lifecycle",
					Arch:    arch,
					NoCache: true,
				},
			})
		})
		t.Run(fmt.Sprintf("xbps/%s/upgrade", arch), func(t *testing.T) {
			t.Parallel()
			acceptWithOldPackage(t, "xbps_upgrade", "xbps", arch,
				"xbps.upgrade.v1.yaml",
				"xbps.upgrade.v2.yaml",
				dockerParams{
					File:    "xbps.dockerfile",
					Target:  "upgrade",
					Arch:    arch,
					NoCache: true,
				},
			)
		})
		t.Run(fmt.Sprintf("xbps/%s/preserve", arch), func(t *testing.T) {
			t.Parallel()
			acceptWithOldPackage(t, "xbps_preserve", "xbps", arch,
				"xbps.preserve.v1.yaml",
				"xbps.preserve.v2.yaml",
				dockerParams{
					File:    "xbps.dockerfile",
					Target:  "preserve",
					Arch:    arch,
					NoCache: true,
				},
			)
		})
		t.Run(fmt.Sprintf("xbps/%s/metadata", arch), func(t *testing.T) {
			t.Parallel()
			accept(t, acceptParms{
				Name:   fmt.Sprintf("metadata_%s", arch),
				Conf:   "xbps.noarch.yaml",
				Format: "xbps",
				Docker: dockerParams{
					File:    "xbps.dockerfile",
					Target:  "metadata",
					Arch:    arch,
					NoCache: true,
				},
			})
		})
	}
}

func TestMSIXStructure(t *testing.T) {
	t.Parallel()
	for _, arch := range []string{"amd64", "arm64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			t.Parallel()

			configFile := "./testdata/acceptance/msix.basic.yaml"
			envFunc := func(s string) string {
				switch s {
				case "BUILD_ARCH":
					return arch
				case "SEMVER":
					return "v1.0.0-0.1.b1+git.abcdefgh"
				default:
					return os.Getenv(s)
				}
			}

			config, err := nfpm.ParseFileWithEnvMapping(configFile, envFunc)
			require.NoError(t, err)

			info, err := config.Get("msix")
			require.NoError(t, err)
			require.NoError(t, nfpm.Validate(info))

			pkg, err := nfpm.Get("msix")
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, pkg.Package(nfpm.WithDefaults(info), &buf))

			// Open the MSIX as a ZIP archive and verify structure
			reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			require.NoError(t, err)

			fileNames := make(map[string]bool)
			for _, f := range reader.File {
				fileNames[f.Name] = true
			}

			// Verify required MSIX structure files exist
			require.True(t, fileNames["AppxManifest.xml"], "AppxManifest.xml must exist")
			require.True(t, fileNames["AppxBlockMap.xml"], "AppxBlockMap.xml must exist")
			require.True(t, fileNames["[Content_Types].xml"], "[Content_Types].xml must exist")

			// Verify payload file exists
			require.True(t, fileNames["app/fake.exe"], "payload file app/fake.exe must exist")
		})
	}
}
