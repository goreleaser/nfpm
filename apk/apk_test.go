package apk

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
)

// nolint: gochecknoglobals
var update = flag.Bool("update", false, "update apk .golden files")

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "v1.0.0",
		Release:     "r1",
		Section:     "default",
		Homepage:    "http://carlosbecker.com",
		Vendor:      "nope",
		Overridables: nfpm.Overridables{
			Depends: []string{
				"bash",
				"foo",
			},
			Recommends: []string{
				"git",
				"bar",
			},
			Suggests: []string{
				"bash",
				"lala",
			},
			Replaces: []string{
				"svn",
				"subversion",
			},
			Provides: []string{
				"bzr",
				"zzz",
			},
			Conflicts: []string{
				"zsh",
				"foobarsh",
			},
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/usr/local/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/usr/share/doc/fake/fake.txt",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        "config",
				},
			},
			EmptyFolders: []string{
				"/var/log/whatever",
				"/usr/share/whatever",
			},
		},
	})
}

func TestArchToAlpine(t *testing.T) {
	verifyArch(t, "", "x86_64")
	verifyArch(t, "abc", "abc")
	verifyArch(t, "386", "x86")
	verifyArch(t, "amd64", "x86_64")
	verifyArch(t, "arm", "armhf")
	verifyArch(t, "arm6", "armhf")
	verifyArch(t, "arm7", "armhf")
	verifyArch(t, "arm64", "aarch64")
}

func verifyArch(t *testing.T, nfpmArch, expectedArch string) {
	t.Helper()
	info := exampleInfo()
	info.Arch = nfpmArch

	assert.NoError(t, Default.Package(info, ioutil.Discard))
	assert.Equal(t, expectedArch, info.Arch)
}

func TestCreateBuilderData(t *testing.T) {
	info := exampleInfo()
	err := info.Validate()
	require.NoError(t, err)
	size := int64(0)
	builderData := createBuilderData(info, &size)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	assert.NoError(t, builderData(tw))

	assert.Equal(t, 11784, buf.Len())
}

func TestCombineToApk(t *testing.T) {
	var bufData bytes.Buffer
	bufData.Write([]byte{1})

	var bufControl bytes.Buffer
	bufControl.Write([]byte{2})

	var bufTarget bytes.Buffer

	assert.NoError(t, combineToApk(&bufTarget, &bufData, &bufControl))
	assert.Equal(t, 2, bufTarget.Len())
}

func TestPathsToCreate(t *testing.T) {
	for pathToTest, parts := range map[string][]string{
		"/usr/share/doc/whatever/foo.md": {"usr", "usr/share", "usr/share/doc", "usr/share/doc/whatever"},
		"/var/moises":                    {"var"},
		"/":                              []string(nil),
	} {
		parts := parts
		pathToTest := pathToTest
		t.Run(fmt.Sprintf("pathToTest: '%s'", pathToTest), func(t *testing.T) {
			assert.Equal(t, parts, pathsToCreate(pathToTest))
		})
	}
}

func TestDefaultWithArch(t *testing.T) {
	expectedChecksums := map[string]string{
		"usr/share/doc/fake/fake.txt": "96c335dc28122b5f09a4cef74b156cd24c23784c",
		"usr/local/bin/fake":          "f46cece3eeb7d9ed5cb244d902775427be71492d",
		"etc/fake/fake.conf":          "96c335dc28122b5f09a4cef74b156cd24c23784c",
	}
	for _, arch := range []string{"386", "amd64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch

			var f bytes.Buffer
			require.NoError(t, Default.Package(info, &f))

			gz, err := gzip.NewReader(&f)
			require.NoError(t, err)
			defer gz.Close()
			tr := tar.NewReader(gz)

			for {
				hdr, err := tr.Next()
				if errors.Is(err, io.EOF) {
					break // End of archive
				}
				require.NoError(t, err)

				require.Equal(t, expectedChecksums[hdr.Name], hdr.PAXRecords["APK-TOOLS.checksum.SHA1"], hdr.Name)
			}
		})
	}
}

func TestNoInfo(t *testing.T) {
	err := Default.Package(nfpm.WithDefaults(&nfpm.Info{}), ioutil.Discard)
	assert.Error(t, err)
}

func TestFileDoesNotExist(t *testing.T) {
	err := Default.Package(
		nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends: []string{
					"bash",
				},
				Contents: []*files.Content{
					{
						Source:      "../testdata/fake",
						Destination: "/usr/local/bin/fake",
					},
					{
						Source:      "../testdata/whatever.confzzz",
						Destination: "/etc/fake/fake.conf",
						Type:        "config",
					},
				},
			},
		}),
		ioutil.Discard,
	)
	assert.EqualError(t, err, "matching \"../testdata/whatever.confzzz\": file does not exist")
}

func TestNoFiles(t *testing.T) {
	err := Default.Package(
		nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends: []string{
					"bash",
				},
			},
		}),
		ioutil.Discard,
	)
	assert.NoError(t, err)
}

func TestCreateBuilderControl(t *testing.T) {
	info := exampleInfo()
	size := int64(12345)
	err := info.Validate()
	require.NoError(t, err)
	builderControl := createBuilderControl(info, size, sha256.New().Sum(nil))

	var w bytes.Buffer
	tw := tar.NewWriter(&w)
	assert.NoError(t, builderControl(tw))

	control := string(extractFromTar(t, w.Bytes(), ".PKGINFO"))
	golden := "testdata/TestCreateBuilderControl.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, []byte(control), 0o655)) // nolint: gosec
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	assert.NoError(t, err)
	assert.Equal(t, string(bts), control)
}

func TestCreateBuilderControlScripts(t *testing.T) {
	info := exampleInfo()
	info.Scripts = nfpm.Scripts{
		PreInstall:  "../testdata/scripts/preinstall.sh",
		PostInstall: "../testdata/scripts/postinstall.sh",
		PreRemove:   "../testdata/scripts/preremove.sh",
		PostRemove:  "../testdata/scripts/postremove.sh",
	}
	err := info.Validate()
	require.NoError(t, err)

	size := int64(12345)
	builderControl := createBuilderControl(info, size, sha256.New().Sum(nil))

	var w bytes.Buffer
	tw := tar.NewWriter(&w)
	assert.NoError(t, builderControl(tw))

	control := string(extractFromTar(t, w.Bytes(), ".PKGINFO"))
	golden := "testdata/TestCreateBuilderControlScripts.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, []byte(control), 0o655)) // nolint: gosec
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	assert.NoError(t, err)
	assert.Equal(t, string(bts), control)
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(),
		InstalledSize: 10,
	}))
	golden := "testdata/TestControl.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o655)) // nolint: gosec
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestSignature(t *testing.T) {
	info := exampleInfo()
	info.APK.Signature.KeyFile = "../internal/sign/testdata/rsa.priv"
	info.APK.Signature.KeyName = "testkey.rsa.pub"
	info.APK.Signature.KeyPassphrase = "hunter2"
	err := info.Validate()
	require.NoError(t, err)

	digest := sha1.New().Sum(nil) // nolint:gosec

	var signatureTarGz bytes.Buffer
	tw := tar.NewWriter(&signatureTarGz)
	require.NoError(t, createSignatureBuilder(digest, info)(tw))

	signature := extractFromTar(t, signatureTarGz.Bytes(), ".SIGN.RSA.testkey.rsa.pub")
	err = sign.RSAVerifySHA1Digest(digest, signature, "../internal/sign/testdata/rsa.pub")
	require.NoError(t, err)

	err = Default.Package(info, ioutil.Discard)
	require.NoError(t, err)
}

func TestSignatureError(t *testing.T) {
	info := exampleInfo()
	info.APK.Signature.KeyFile = "../internal/sign/testdata/rsa.priv"
	info.APK.Signature.KeyName = "testkey.rsa.pub"
	info.APK.Signature.KeyPassphrase = "hunter2"
	err := info.Validate()
	require.NoError(t, err)

	// wrong hash format
	digest := sha256.New().Sum(nil)

	var signatureTarGz bytes.Buffer

	err = createSignature(&signatureTarGz, info, digest)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.True(t, errors.As(err, &expectedError))

	info.APK.Signature.KeyName = ""
	info.Maintainer = ""
	digest = sha1.New().Sum(nil) // nolint:gosec
	err = createSignature(&signatureTarGz, info, digest)
	require.True(t, errors.As(err, &expectedError))
}

func TestDisableGlobbing(t *testing.T) {
	info := exampleInfo()
	info.DisableGlobbing = true
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/{file}[",
			Destination: "/test/{file}[",
		},
	}
	err := info.Validate()
	require.NoError(t, err)

	size := int64(0)
	var dataTarGz bytes.Buffer
	_, err = createData(&dataTarGz, info, &size)
	require.NoError(t, err)

	gzr, err := gzip.NewReader(&dataTarGz)
	require.NoError(t, err)
	dataTar, err := ioutil.ReadAll(gzr)
	require.NoError(t, err)

	extractedContent := extractFromTar(t, dataTar, "test/{file}[")
	actualContent, err := ioutil.ReadFile("../testdata/{file}[")
	require.NoError(t, err)
	require.Equal(t, actualContent, extractedContent)
}

func extractFromTar(t *testing.T, tarFile []byte, fileName string) []byte {
	t.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		if hdr.Name != fileName {
			continue
		}

		data, err := ioutil.ReadAll(tr)
		require.NoError(t, err)
		return data
	}

	t.Fatalf("file %q not found in tar file", fileName)
	return nil
}

func TestAPKConventionalFileName(t *testing.T) {
	apkName := "default"
	testCases := []struct {
		Arch       string
		Version    string
		Meta       string
		Release    string
		Prerelease string
		Expect     string
	}{
		{
			Arch: "amd64", Version: "1.2.3",
			Expect: "default_1.2.3_x86_64.apk",
		},
		{
			Arch: "386", Version: "1.2.3", Meta: "git",
			Expect: "default_1.2.3+git_x86.apk",
		},
		{
			Arch: "386", Version: "1.2.3", Meta: "git", Release: "1",
			Expect: "default_1.2.3-1+git_x86.apk",
		},
		{
			Arch: "all", Version: "1.2.3",
			Expect: "default_1.2.3_all.apk",
		},
		{
			Arch: "386", Version: "1.2.3", Release: "1", Prerelease: "5",
			Expect: "default_1.2.3-1~5_x86.apk",
		},
	}

	for _, testCase := range testCases {
		info := &nfpm.Info{
			Name:            apkName,
			Arch:            testCase.Arch,
			Version:         testCase.Version,
			VersionMetadata: testCase.Meta,
			Release:         testCase.Release,
			Prerelease:      testCase.Prerelease,
		}
		assert.Equal(t, testCase.Expect, Default.ConventionalFileName(info))
	}
}

func TestPackageSymlinks(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/fake",
			Destination: "fake",
			Type:        "symlink",
		},
	}
	assert.NoError(t, Default.Package(info, ioutil.Discard))
}
