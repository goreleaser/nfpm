package arch

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/pgzip"
	"github.com/stretchr/testify/require"
)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo-test",
		Arch:        "amd64",
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "1.0.0",
		Prerelease:  "beta-1",
		Section:     "default",
		Homepage:    "http://carlosbecker.com",
		Vendor:      "nope",
		License:     "MIT",
		Overridables: nfpm.Overridables{
			Depends: []string{
				"bash",
			},
			Replaces: []string{
				"svn",
			},
			Provides: []string{
				"bzr",
			},
			Conflicts: []string{
				"zsh",
			},
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/usr/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        files.TypeConfig,
				},
				{
					Destination: "/var/log/whatever",
					Type:        files.TypeDir,
				},
				{
					Destination: "/usr/share/whatever",
					Type:        files.TypeDir,
				},
				{
					Source:      "/etc/fake/fake.conf",
					Destination: "/etc/fake/fake-link.conf",
					Type:        files.TypeSymlink,
				},
				{
					Source:      "../testdata/something",
					Destination: "/etc/something",
				},
			},
			Scripts: nfpm.Scripts{
				PreInstall:  "../testdata/scripts/preinstall.sh",
				PostInstall: "../testdata/scripts/postinstall.sh",
				PreRemove:   "../testdata/scripts/preremove.sh",
				PostRemove:  "../testdata/scripts/postremove.sh",
			},
			ArchLinux: nfpm.ArchLinux{
				Scripts: nfpm.ArchLinuxScripts{
					PreUpgrade:  "../testdata/scripts/preupgrade.sh",
					PostUpgrade: "../testdata/scripts/postupgrade.sh",
				},
			},
		},
	})
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".pkg.tar.zst", Default.ConventionalExtension())
}

func TestArch(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch
			err := Default.Package(info, io.Discard)
			require.NoError(t, err)
		})
	}
}

func TestArchPlatform(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.pkg.tar.zstd")
	require.NoError(t, err)
	info := exampleInfo()
	info.Platform = "darwin"
	err = Default.Package(info, f)
	require.Error(t, err)
}

func TestArchNoFiles(t *testing.T) {
	info := exampleInfo()
	info.Contents = nil
	info.Scripts = nfpm.Scripts{}
	info.ArchLinux = nfpm.ArchLinux{}
	err := Default.Package(info, io.Discard)
	require.NoError(t, err)
}

func TestArchNoInfo(t *testing.T) {
	err := Default.Package(nfpm.WithDefaults(&nfpm.Info{}), io.Discard)
	require.Error(t, err)
}

func TestArchConventionalFileName(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch
			name := Default.ConventionalFileName(info)
			require.Equal(t,
				"foo-test-1.0.0beta_1-1-"+archToArchLinux[arch]+".pkg.tar.zst",
				name,
			)
		})
	}
}

func TestArchPkginfo(t *testing.T) {
	info := exampleInfo()
	pkginfoData, err := makeTestPkginfo(t, info)
	require.NoError(t, err)
	fields := extractPkginfoFields(pkginfoData)
	require.Equal(t, "foo-test", fields["pkgname"])
	require.Equal(t, "foo-test", fields["pkgbase"])
	require.Equal(t, "1.0.0-1", fields["pkgver"])
	require.Equal(t, "Foo does things", fields["pkgdesc"])
	require.Equal(t, "http://carlosbecker.com", fields["url"])
	require.Equal(t, "Unknown Packager", fields["packager"])
	require.Equal(t, "x86_64", fields["arch"])
	require.Equal(t, "MIT", fields["license"])
	require.Equal(t, "1234", fields["size"])
	require.Equal(t, "svn", fields["replaces"])
	require.Equal(t, "zsh", fields["conflict"])
	require.Equal(t, "bzr", fields["provides"])
	require.Equal(t, "bash", fields["depend"])
	require.Equal(t, "etc/fake/fake.conf", fields["backup"])
}

func TestArchPkgbase(t *testing.T) {
	info := exampleInfo()
	info.ArchLinux.Pkgbase = "foo"
	pkginfoData, err := makeTestPkginfo(t, info)
	require.NoError(t, err)
	fields := extractPkginfoFields(pkginfoData)
	require.Equal(t, "foo", fields["pkgbase"])
}

func TestArchInvalidName(t *testing.T) {
	info := exampleInfo()
	info.Name = "#"
	_, err := makeTestPkginfo(t, info)
	require.ErrorIs(t, err, ErrInvalidPkgName)
}

func TestArchVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "0.0.1"
	info.Release = "4"
	pkginfoData, err := makeTestPkginfo(t, info)
	require.NoError(t, err)
	fields := extractPkginfoFields(pkginfoData)
	require.Equal(t, "0.0.1-4", fields["pkgver"])
}

func TestArchVersionWithEpoch(t *testing.T) {
	info := exampleInfo()
	info.Version = "0.0.1"
	info.Epoch = "2"
	pkginfoData, err := makeTestPkginfo(t, info)
	require.NoError(t, err)
	fields := extractPkginfoFields(pkginfoData)
	require.Equal(t, "2:0.0.1beta_1-1", fields["pkgver"])
}

func TestArchOverrideArchitecture(t *testing.T) {
	info := exampleInfo()
	info.ArchLinux.Arch = "randomarch"
	pkginfoData, err := makeTestPkginfo(t, info)
	require.NoError(t, err)
	fields := extractPkginfoFields(pkginfoData)
	require.Equal(t, "randomarch", fields["arch"])
}

func makeTestPkginfo(t *testing.T, info *nfpm.Info) ([]byte, error) {
	t.Helper()

	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	entry, err := createPkginfo(info, tw, 1234)
	if err != nil {
		return nil, err
	}

	tw.Close()

	tr := tar.NewReader(buf)
	_, err = tr.Next()
	require.NoError(t, err)

	pkginfoData := make([]byte, entry.Size)
	_, err = io.ReadFull(tr, pkginfoData)
	if err != nil {
		return nil, err
	}

	return pkginfoData, nil
}

func extractPkginfoFields(data []byte) map[string]string {
	strData := string(data)
	strData = strings.TrimPrefix(strData, "# Generated by nfpm\n")
	strData = strings.TrimSpace(strData)

	splitData := strings.Split(strData, "\n")
	out := map[string]string{}

	for _, kvPair := range splitData {
		splitPair := strings.Split(kvPair, " = ")
		out[splitPair[0]] = splitPair[1]
	}

	return out
}

const correctMtree = `#mtree
./foo/bar time=1234.0 mode=755 type=dir
./foo/bar/file time=1234.0 mode=600 size=143 type=file md5digest=abcd sha256digest=ef12
./3 time=12345.0 mode=644 size=100 type=file md5digest=abcd sha256digest=ef12
./sh time=123456.0 mode=777 type=link link=/bin/bash
`

func TestArchMtree(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	err := createMtree(tw, []MtreeEntry{
		{
			Destination: "foo/bar",
			Time:        1234,
			Type:        files.TypeDir,
			Mode:        0o755,
		},
		{
			Destination: "foo/bar/file",
			Time:        1234,
			Type:        files.TypeFile,
			Mode:        0o600,
			Size:        143,
			MD5:         []byte{0xAB, 0xCD},
			SHA256:      []byte{0xEF, 0x12},
		},
		{
			Destination: "3",
			Time:        12345,
			Mode:        0o644,
			Size:        100,
			Type:        files.TypeFile,
			MD5:         []byte{0xAB, 0xCD},
			SHA256:      []byte{0xEF, 0x12},
		},
		{
			LinkSource:  "/bin/bash",
			Destination: "sh",
			Time:        123456,
			Mode:        0o777,
			Type:        files.TypeSymlink,
		},
	})
	require.NoError(t, err)

	tw.Close()

	tr := tar.NewReader(buf)
	_, err = tr.Next()
	require.NoError(t, err)

	gr, err := pgzip.NewReader(tr)
	require.NoError(t, err)
	defer gr.Close()

	mtree, err := io.ReadAll(gr)
	require.NoError(t, err)

	require.Equal(t, correctMtree, string(mtree))
}

func TestGlob(t *testing.T) {
	var pkg bytes.Buffer
	require.NoError(t, Default.Package(nfpm.WithDefaults(&nfpm.Info{
		Name:       "nfpm-repro",
		Version:    "1.0.0",
		Maintainer: "asdfasdf",

		Overridables: nfpm.Overridables{
			Contents: files.Contents{
				{
					Destination: "/usr/share/nfpm-repro",
					Source:      "../files/*.go",
				},
			},
		},
	}), &pkg))

	pkgZstd, err := zstd.NewReader(&pkg)
	require.NoError(t, err)
	t.Cleanup(func() { pkgZstd.Close() })
	pkgTar := tar.NewReader(pkgZstd)
	for {
		f, err := pkgTar.Next()
		if err == io.EOF || f == nil {
			break
		}

		if f.Name == ".MTREE" {
			break
		}
	}

	mtreeTarBts, err := io.ReadAll(pkgTar)
	require.NoError(t, err)

	mtreeGzip, err := pgzip.NewReader(bytes.NewReader(mtreeTarBts))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mtreeGzip.Close()) })

	mtreeContentBts, err := io.ReadAll(mtreeGzip)
	require.NoError(t, err)

	expected := map[string][]string{
		"./.PKGINFO":                           {"mode=644", "size=188", "type=file"},
		"./usr/":                               {"mode=755", "type=dir"},
		"./usr/share/":                         {"mode=755", "type=dir"},
		"./usr/share/nfpm-repro/":              {"mode=755", "type=dir"},
		"./usr/share/nfpm-repro/files.go":      {"mode=664", "size=15900", "type=file", "md5digest=0c4e339bc678ab6b2b291ec3088e32a2", "sha256digest=27614421b69216e9d8ddd177210f2c9a71d8ff9d76cc2b1654276eccd04e84c2"},
		"./usr/share/nfpm-repro/files_test.go": {"mode=664", "size=17741", "type=file", "md5digest=52818697d19e885bc2e5b37f2dc4a7fb", "sha256digest=fdbb35f8c11e118508ddd526c0a7797171ea100aa14337caddd7ab249ee91272"},
	}

	for _, line := range strings.Split(string(mtreeContentBts), "\n") {
		if line == "#mtree" || line == "" {
			continue
		}
		parts := strings.Fields(line)
		filename := parts[0]
		expect := expected[filename]
		modTime := parts[1]
		require.Regexp(t, regexp.MustCompile(`time=\d+\.\d`), modTime)
		require.Equal(t, expect, parts[2:len(expect)+2], filename)
	}
}
