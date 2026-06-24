package rpm

import (
	"bytes"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/sassoftware/go-rpmutils"
	"github.com/stretchr/testify/require"
)

func TestSRPMConventionalExtension(t *testing.T) {
	require.Equal(t, ".src.rpm", DefaultSRPM.ConventionalExtension())
}

func TestSRPMConventionalFileName(t *testing.T) {
	info := exampleInfo()
	// Source packages are named without an architecture component.
	require.Equal(t, "foo-1.0.0-1.src.rpm", DefaultSRPM.ConventionalFileName(info))
}

func TestSRPM(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, DefaultSRPM.Package(exampleInfo(), &buf))

	rpm, err := rpmutils.ReadRpm(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	// A source package uses arch "src" regardless of the configured target arch.
	arch, err := rpm.Header.GetString(rpmutils.ARCH)
	require.NoError(t, err)
	require.Equal(t, "src", arch)

	osName, err := rpm.Header.GetString(rpmutils.OS)
	require.NoError(t, err)
	require.Equal(t, "linux", osName)

	version, err := rpm.Header.GetString(rpmutils.VERSION)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", version)

	release, err := rpm.Header.GetString(rpmutils.RELEASE)
	require.NoError(t, err)
	require.Equal(t, "1", release)

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	require.Equal(t, "foo", group)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	require.Equal(t, "Foo does things", summary)

	// It is marked as a source package and omits SOURCERPM.
	srcPkg, err := rpm.Header.GetUint32s(tagSourcePackage)
	require.NoError(t, err)
	require.Equal(t, []uint32{1}, srcPkg)

	_, err = rpm.Header.GetString(rpmutils.SOURCERPM)
	require.Error(t, err, "source packages must not set SOURCERPM")

	// The payload contains the generated spec and the bundled source tarball.
	tree := getTree(t, buf.Bytes())
	require.Contains(t, tree, "/foo.spec")
	require.Contains(t, tree, "/foo-1.0.0.tar.gz")
}

func TestSRPMArchAlwaysSrc(t *testing.T) {
	info := exampleInfo()
	info.Arch = "riscv64"

	var buf bytes.Buffer
	require.NoError(t, DefaultSRPM.Package(info, &buf))

	rpm, err := rpmutils.ReadRpm(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	arch, err := rpm.Header.GetString(rpmutils.ARCH)
	require.NoError(t, err)
	require.Equal(t, "src", arch)

	// The target architecture is carried by the spec's BuildArch instead.
	spec, err := extractFileFromRpm(buf.Bytes(), "/foo.spec")
	require.NoError(t, err)
	require.Contains(t, string(spec), "BuildArch: "+archToRPM["riscv64"])
}

func TestSRPMSpecContents(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, DefaultSRPM.Package(exampleInfo(), &buf))

	specBytes, err := extractFileFromRpm(buf.Bytes(), "/foo.spec")
	require.NoError(t, err)
	spec := string(specBytes)

	// Metadata preamble.
	require.Contains(t, spec, "Name: foo")
	require.Contains(t, spec, "Version: 1.0.0")
	require.Contains(t, spec, "Release: 1")
	require.Contains(t, spec, "Epoch: 0")
	require.Contains(t, spec, "BuildArch: "+archToRPM["amd64"])
	require.Contains(t, spec, "License: MIT")
	require.Contains(t, spec, "Group: foo")

	// Dependencies.
	require.Contains(t, spec, "Requires: bash")
	require.Contains(t, spec, "Provides: bzr")
	require.Contains(t, spec, "Conflicts: zsh")
	require.Contains(t, spec, "Obsoletes: svn")
	require.Contains(t, spec, "Recommends: git")

	// Reproducible-rebuild pragmas and the tarball-extracting %install.
	require.Contains(t, spec, "%global debug_package %{nil}")
	require.Contains(t, spec, "AutoReqProv: no")
	require.Contains(t, spec, "Source0: foo-1.0.0.tar.gz")
	require.Contains(t, spec, "tar -C %{buildroot} -xf %{SOURCE0}")

	// %files entries with directives.
	require.Contains(t, spec, "/usr/bin/fake")
	require.Contains(t, spec, "%config %attr")
	require.Contains(t, spec, "/etc/fake/fake.conf")
	require.Contains(t, spec, "%dir %attr")

	// Scriptlets are inlined verbatim.
	require.Contains(t, spec, "%pre\n")
	require.Contains(t, spec, "%post\n")
	require.Contains(t, spec, "%pretrans\n")
	require.Contains(t, spec, "%posttrans\n")
	require.Contains(t, spec, "%verifyscript\n")
	require.Contains(t, spec, `echo "Preinstall"`)
}

func TestSRPMGenerateSpecFileDirectives(t *testing.T) {
	info := nfpm.WithDefaults(&nfpm.Info{
		Name:        "spectest",
		Arch:        "amd64",
		Version:     "2.0.0",
		Description: "spec directive coverage",
		Maintainer:  "maintainer",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{Source: "../testdata/fake", Destination: "/usr/bin/fake"},
				{Source: "../testdata/whatever.conf", Destination: "/etc/cfg.conf", Type: files.TypeConfigNoReplace},
				{Source: "../testdata/fake", Destination: "/usr/share/doc/readme", Type: files.TypeRPMDoc},
				{Destination: "/var/log/ghost.log", Type: files.TypeRPMGhost},
				{Source: "/usr/bin/fake", Destination: "/usr/bin/fakelink", Type: files.TypeSymlink},
				{Destination: "/var/lib/spectest", Type: files.TypeDir},
			},
		},
	})
	info = setDefaults(info)
	require.NoError(t, nfpm.PrepareForPackager(info, "rpm"))

	spec, err := generateSpec(info, "spectest-2.0.0.tar.gz")
	require.NoError(t, err)

	require.Contains(t, spec, "%config(noreplace) %attr")
	require.Contains(t, spec, "/etc/cfg.conf")
	require.Contains(t, spec, "%doc %attr")
	require.Contains(t, spec, "%ghost %attr")
	require.Contains(t, spec, "/var/log/ghost.log")
	require.Contains(t, spec, "%dir %attr")
	require.Contains(t, spec, "/var/lib/spectest")
	require.Contains(t, spec, `%attr(-, root, root) "/usr/bin/fakelink"`)
}
