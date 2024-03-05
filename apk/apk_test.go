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
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
	"github.com/stretchr/testify/require"
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
		Prerelease:  "beta1",
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
					Destination: "/usr/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/usr/share/doc/fake/fake.txt",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        files.TypeConfig,
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake2.conf",
					Type:        files.TypeConfigNoReplace,
				},
				{
					Destination: "/var/log/whatever",
					Type:        files.TypeDir,
				},
				{
					Destination: "/usr/share/whatever",
					Type:        files.TypeDir,
				},
			},
		},
	})
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".apk", Default.ConventionalExtension())
}

func TestCreateBuilderData(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, "apk"))
	size := int64(0)
	builderData := createBuilderData(info, &size)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	require.NoError(t, builderData(tw))

	require.Equal(t, 13824, buf.Len(), buf.String())
}

func TestCombineToApk(t *testing.T) {
	var bufData bytes.Buffer
	bufData.Write([]byte{1})

	var bufControl bytes.Buffer
	bufControl.Write([]byte{2})

	var bufTarget bytes.Buffer

	require.NoError(t, combineToApk(&bufTarget, &bufData, &bufControl))
	require.Equal(t, 2, bufTarget.Len())
}

func TestDefaultWithArch(t *testing.T) {
	expectedChecksums := map[string]string{
		"usr/share/doc/fake/fake.txt": "96c335dc28122b5f09a4cef74b156cd24c23784c",
		"usr/bin/fake":                "f46cece3eeb7d9ed5cb244d902775427be71492d",
		"etc/fake/fake.conf":          "96c335dc28122b5f09a4cef74b156cd24c23784c",
		"etc/fake/fake2.conf":         "96c335dc28122b5f09a4cef74b156cd24c23784c",
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

func TestApkPlatform(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.apk")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	info := exampleInfo()
	info.Platform = "darwin"
	err = Default.Package(info, f)
	require.Error(t, err)
}

func TestNoInfo(t *testing.T) {
	err := Default.Package(nfpm.WithDefaults(&nfpm.Info{}), io.Discard)
	require.Error(t, err)
}

func TestFileDoesNotExist(t *testing.T) {
	abs, err := filepath.Abs("../testdata/whatever.confzzz")
	require.NoError(t, err)
	err = Default.Package(
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
						Destination: "/usr/bin/fake",
					},
					{
						Source:      "../testdata/whatever.confzzz",
						Destination: "/etc/fake/fake.conf",
						Type:        files.TypeConfig,
					},
				},
			},
		}),
		io.Discard,
	)
	require.EqualError(t, err, fmt.Sprintf("matching \"%s\": file does not exist", filepath.ToSlash(abs)))
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
		io.Discard,
	)
	require.NoError(t, err)
}

func TestCreateBuilderControl(t *testing.T) {
	info := exampleInfo()
	size := int64(12345)
	err := nfpm.PrepareForPackager(info, "apk")
	require.NoError(t, err)
	builderControl := createBuilderControl(info, size, sha256.New().Sum(nil))

	var w bytes.Buffer
	tw := tar.NewWriter(&w)
	require.NoError(t, builderControl(tw))

	control := string(extractFromTar(t, w.Bytes(), ".PKGINFO"))
	golden := "testdata/TestCreateBuilderControl.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(control), 0o655)) // nolint: gosec
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), control)
}

func TestCreateBuilderControlScripts(t *testing.T) {
	info := exampleInfo()
	info.Scripts = nfpm.Scripts{
		PreInstall:  "../testdata/scripts/preinstall.sh",
		PostInstall: "../testdata/scripts/postinstall.sh",
		PreRemove:   "../testdata/scripts/preremove.sh",
		PostRemove:  "../testdata/scripts/postremove.sh",
	}
	info.APK.Scripts = nfpm.APKScripts{
		PreUpgrade:  "../testdata/scripts/preupgrade.sh",
		PostUpgrade: "../testdata/scripts/postupgrade.sh",
	}
	err := nfpm.PrepareForPackager(info, "apk")
	require.NoError(t, err)

	size := int64(12345)
	builderControl := createBuilderControl(info, size, sha256.New().Sum(nil))

	var w bytes.Buffer
	tw := tar.NewWriter(&w)
	require.NoError(t, builderControl(tw))

	control := string(extractFromTar(t, w.Bytes(), ".PKGINFO"))
	golden := "testdata/TestCreateBuilderControlScripts.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(control), 0o655)) // nolint: gosec
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), control)

	// Validate scripts are correct
	script := string(extractFromTar(t, w.Bytes(), ".pre-install"))
	require.Contains(t, script, `echo "Preinstall" > /dev/null`)
	script = string(extractFromTar(t, w.Bytes(), ".post-install"))
	require.Contains(t, script, `echo "Postinstall" > /dev/null`)
	script = string(extractFromTar(t, w.Bytes(), ".pre-upgrade"))
	require.Contains(t, script, `echo "PreUpgrade" > /dev/null`)
	script = string(extractFromTar(t, w.Bytes(), ".post-upgrade"))
	require.Contains(t, script, `echo "PostUpgrade" > /dev/null`)
	script = string(extractFromTar(t, w.Bytes(), ".pre-deinstall"))
	require.Contains(t, script, `echo "Preremove" > /dev/null`)
	script = string(extractFromTar(t, w.Bytes(), ".post-deinstall"))
	require.Contains(t, script, `echo "Postremove" > /dev/null`)
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(),
		InstalledSize: 10,
	}))
	golden := "testdata/TestControl.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o655)) // nolint: gosec
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestSignature(t *testing.T) {
	info := exampleInfo()
	info.APK.Signature.KeyFile = "../internal/sign/testdata/rsa.priv"
	info.APK.Signature.KeyName = "testkey.rsa.pub"
	info.APK.Signature.KeyPassphrase = "hunter2"
	err := nfpm.PrepareForPackager(info, "apk")
	require.NoError(t, err)

	digest := sha1.New().Sum(nil) // nolint:gosec

	var signatureTarGz bytes.Buffer
	tw := tar.NewWriter(&signatureTarGz)
	require.NoError(t, createSignatureBuilder(digest, info)(tw))

	signature := extractFromTar(t, signatureTarGz.Bytes(), ".SIGN.RSA.testkey.rsa.pub")
	err = sign.RSAVerifySHA1Digest(digest, signature, "../internal/sign/testdata/rsa.pub")
	require.NoError(t, err)

	err = Default.Package(info, io.Discard)
	require.NoError(t, err)
}

func TestSignatureError(t *testing.T) {
	info := exampleInfo()
	info.APK.Signature.KeyFile = "../internal/sign/testdata/rsa.priv"
	info.APK.Signature.KeyName = "testkey.rsa.pub"
	info.APK.Signature.KeyPassphrase = "hunter2"
	err := nfpm.PrepareForPackager(info, "apk")
	require.NoError(t, err)

	// wrong hash format
	digest := sha256.New().Sum(nil)

	var signatureTarGz bytes.Buffer

	err = createSignature(&signatureTarGz, info, digest)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &expectedError)

	info.APK.Signature.KeyName = ""
	info.Maintainer = ""
	digest = sha1.New().Sum(nil) // nolint:gosec
	err = createSignature(&signatureTarGz, info, digest)
	require.ErrorAs(t, err, &expectedError)
}

func TestSignatureCallback(t *testing.T) {
	info := exampleInfo()
	info.APK.Signature.SignFn = func(r io.Reader) ([]byte, error) {
		digest, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return sign.RSASignSHA1Digest(digest, "../internal/sign/testdata/rsa.priv", "hunter2")
	}
	info.APK.Signature.KeyName = "testkey.rsa.pub"
	err := nfpm.PrepareForPackager(info, "apk")
	require.NoError(t, err)

	digest := sha1.New().Sum(nil) // nolint:gosec

	var signatureTarGz bytes.Buffer
	tw := tar.NewWriter(&signatureTarGz)
	require.NoError(t, createSignatureBuilder(digest, info)(tw))

	signature := extractFromTar(t, signatureTarGz.Bytes(), ".SIGN.RSA.testkey.rsa.pub")
	err = sign.RSAVerifySHA1Digest(digest, signature, "../internal/sign/testdata/rsa.pub")
	require.NoError(t, err)

	err = Default.Package(info, io.Discard)
	require.NoError(t, err)
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
	err := nfpm.PrepareForPackager(info, "apk")
	require.NoError(t, err)

	size := int64(0)
	var dataTarGz bytes.Buffer
	_, err = createData(&dataTarGz, info, &size)
	require.NoError(t, err)

	gzr, err := gzip.NewReader(&dataTarGz)
	require.NoError(t, err)
	dataTar, err := io.ReadAll(gzr)
	require.NoError(t, err)

	extractedContent := extractFromTar(t, dataTar, "test/{file}[")
	actualContent, err := os.ReadFile("../testdata/{file}[")
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

		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		return data
	}

	t.Fatalf("file %q not found in tar file", fileName)
	return nil
}

func tarContents(tb testing.TB, tarFile []byte) []string {
	tb.Helper()

	contents := []string{}

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		contents = append(contents, hdr.Name)
	}

	return contents
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
			Expect: "default_1.2.3-git_x86.apk",
		},
		{
			Arch: "386", Version: "1.2.3", Meta: "1", Release: "10",
			Expect: "default_1.2.3-r10-p1_x86.apk",
		},
		{
			Arch: "386", Version: "1.2.3", Prerelease: "git", Release: "1",
			Expect: "default_1.2.3_git-r1_x86.apk",
		},
		{
			Arch: "all", Version: "1.2.3",
			Expect: "default_1.2.3_all.apk",
		},
		{
			Arch: "386", Version: "1.2.3", Release: "1", Prerelease: "beta1",
			Expect: "default_1.2.3_beta1-r1_x86.apk",
		},
		{
			Arch: "amd64", Version: "1.2.3a", Prerelease: "alpha1", Release: "47", Meta: "git-aaaccc",
			Expect: "default_1.2.3a_alpha1-r47-git-aaaccc_x86_64.apk",
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
		require.Equal(t, testCase.Expect, Default.ConventionalFileName(info))
	}
}

func TestPackageSymlinks(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/fake",
			Destination: "fake",
			Type:        files.TypeSymlink,
		},
	}
	require.NoError(t, Default.Package(info, io.Discard))
}

func TestDirectories(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/foo/file",
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/bar/file",
		},
		{
			Destination: "/etc/bar",
			Type:        files.TypeDir,
			FileInfo: &files.ContentFileInfo{
				Owner: "test",
				Mode:  0o700,
			},
		},
		{
			Destination: "/etc/baz",
			Type:        files.TypeDir,
		},
		{
			Destination: "/usr/lib/something/somethingelse",
			Type:        files.TypeDir,
		},
	}

	require.NoError(t, nfpm.PrepareForPackager(info, "apk"))

	var buf bytes.Buffer
	size := int64(0)
	err := createFilesInsideTarGz(info, tar.NewWriter(&buf), &size)
	require.NoError(t, err)

	require.Equal(t, []string{
		"etc/",
		"etc/bar/",
		"etc/bar/file",
		"etc/baz/",
		"etc/foo/",
		"etc/foo/file",
		"usr/",
		"usr/lib/",
		"usr/lib/something/",
		"usr/lib/something/somethingelse/",
	}, getTree(t, buf.Bytes()))

	// for apks all implicit or explicit directories are created in the tarball
	h := extractFileHeaderFromTar(t, buf.Bytes(), "/etc")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, buf.Bytes(), "/etc/foo")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, buf.Bytes(), "/etc/bar")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	require.Equal(t, int64(0o700), h.Mode)
	require.Equal(t, "test", h.Uname)
	h = extractFileHeaderFromTar(t, buf.Bytes(), "/etc/baz")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
}

func TestNoDuplicateAutocreatedDirectories(t *testing.T) {
	info := exampleInfo()
	info.DisableGlobbing = true
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/fake",
			Destination: "/etc/foo/bar",
		},
		{
			Type:        files.TypeDir,
			Destination: "/etc/foo",
		},
	}
	require.NoError(t, nfpm.PrepareForPackager(info, "apk"))

	expected := map[string]bool{
		"etc/":        true,
		"etc/foo/":    true,
		"etc/foo/bar": true,
	}

	var buf bytes.Buffer
	size := int64(0)
	err := createFilesInsideTarGz(info, tar.NewWriter(&buf), &size)
	require.NoError(t, err)

	contents := tarContents(t, buf.Bytes())

	if len(expected) != len(contents) {
		t.Fatalf("contents has %d entries instead of %d: %#v", len(contents), len(expected), contents)
	}

	for _, entry := range contents {
		if !expected[entry] {
			t.Fatalf("unexpected content: %q", entry)
		}
	}
}

func TestNoDuplicateDirectories(t *testing.T) {
	info := exampleInfo()
	info.DisableGlobbing = true
	info.Contents = []*files.Content{
		{
			Type:        files.TypeDir,
			Destination: "/etc/foo",
		},
		{
			Type:        files.TypeDir,
			Destination: "/etc/foo/",
		},
	}
	require.Error(t, nfpm.PrepareForPackager(info, "apk"))
}

func TestNoDuplicateContents(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/foo/file",
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/bar/file",
		},
		{
			Destination: "/etc/bar",
			Type:        files.TypeDir,
			FileInfo: &files.ContentFileInfo{
				Owner: "test",
				Mode:  0o700,
			},
		},
		{
			Destination: "/etc/baz",
			Type:        files.TypeDir,
		},
	}

	require.NoError(t, nfpm.PrepareForPackager(info, "apk"))

	var buf bytes.Buffer
	size := int64(0)
	err := createFilesInsideTarGz(info, tar.NewWriter(&buf), &size)
	require.NoError(t, err)

	exists := map[string]bool{}

	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(t, err)

		_, ok := exists[hdr.Name]
		if ok {
			t.Fatalf("%s exists more than once in tarball", hdr.Name)
		}

		exists[hdr.Name] = true
	}
}

func extractFileHeaderFromTar(tb testing.TB, tarFile []byte, filename string) *tar.Header {
	tb.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) != path.Join("/", filename) { // nolint:gosec
			continue
		}

		return hdr
	}

	tb.Fatalf("file %q does not exist in tar", filename)

	return nil
}

func getTree(tb testing.TB, tarFile []byte) []string {
	tb.Helper()

	var result []string
	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		result = append(result, hdr.Name)
	}

	return result
}

func TestArches(t *testing.T) {
	for k := range archToAlpine {
		t.Run(k, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = k
			info = ensureValidArch(info)
			require.Equal(t, archToAlpine[k], info.Arch)
		})
	}

	t.Run("override", func(t *testing.T) {
		info := exampleInfo()
		info.APK.Arch = "foo64"
		info = ensureValidArch(info)
		require.Equal(t, "foo64", info.Arch)
	})
}

func TestGlob(t *testing.T) {
	require.NoError(t, Default.Package(nfpm.WithDefaults(&nfpm.Info{
		Name:       "nfpm-repro",
		Version:    "1.0.0",
		Maintainer: "asdfasdf",

		Overridables: nfpm.Overridables{
			Contents: files.Contents{
				{
					Destination: "/usr/share/nfpm-repro",
					Source:      "../files/*",
				},
			},
		},
	}), io.Discard))
}
