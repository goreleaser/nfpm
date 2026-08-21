package rpm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.digitalxero.dev/rpm"
)

func TestParseCompression(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, tt := range []struct {
			compression string
			concrete    string // unexported concrete compressor type name
		}{
			{"", "gzipCompressor"},
			{"gzip", "gzipCompressor"},
			{"gz", "gzipCompressor"},
			{"gzip:5", "gzipCompressor"},
			{"zstd", "zstdCompressor"},
			{"zstd:19", "zstdCompressor"},
			{"xz", "xzCompressor"},
			{"xz:9", "xzCompressor"}, // level is ignored for xz
			{"lzma", "lzmaCompressor"},
			{"lzma:9", "lzmaCompressor"}, // level is ignored for lzma
		} {
			t.Run(tt.compression, func(t *testing.T) {
				comp, err := parseCompression(tt.compression)
				require.NoError(t, err)
				require.NotNil(t, comp)
				require.Equal(t, tt.concrete, reflect.TypeOf(comp).Name())
			})
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		comp, err := parseCompression("gzip:notanumber")
		require.Error(t, err)
		require.Nil(t, comp)
		require.Contains(t, err.Error(), "invalid compression level")
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		comp, err := parseCompression("bzip2")
		require.Error(t, err)
		require.Nil(t, comp)
		require.Contains(t, err.Error(), "unsupported compression")
	})
}

func TestReadScriptsError(t *testing.T) {
	const missing = "../testdata/does-not-exist.sh"

	// Each lifecycle script is read independently; pointing any single one at a
	// missing file must surface that read error.
	for name, set := range map[string]func(*nfpm.Info){
		"pretrans":    func(i *nfpm.Info) { i.RPM.Scripts.PreTrans = missing },
		"preinstall":  func(i *nfpm.Info) { i.Scripts.PreInstall = missing },
		"preremove":   func(i *nfpm.Info) { i.Scripts.PreRemove = missing },
		"postinstall": func(i *nfpm.Info) { i.Scripts.PostInstall = missing },
		"postremove":  func(i *nfpm.Info) { i.Scripts.PostRemove = missing },
		"posttrans":   func(i *nfpm.Info) { i.RPM.Scripts.PostTrans = missing },
		"verify":      func(i *nfpm.Info) { i.RPM.Scripts.Verify = missing },
	} {
		t.Run(name, func(t *testing.T) {
			info := &nfpm.Info{}
			set(info)

			_, err := readScripts(info)
			require.Error(t, err)
		})
	}
}

func TestApplyRelationsInvalidPostRequire(t *testing.T) {
	info := exampleInfo()
	info.RPM.Requires.Post = []string{"pkg >>> 2"}

	err := applyRelations(rpm.NewPackage(), info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown version operator")
}

func TestReadTriggersErrors(t *testing.T) {
	for name, trigger := range map[string]struct {
		trigger  nfpm.RPMTrigger
		expected string
	}{
		"type":    {nfpm.RPMTrigger{Type: "invalid", Script: "script", Conditions: []string{"foo"}}, "unknown trigger type"},
		"script":  {nfpm.RPMTrigger{Type: "in", Conditions: []string{"foo"}}, "script must be provided"},
		"missing": {nfpm.RPMTrigger{Type: "in", Script: "../testdata/does-not-exist.sh", Conditions: []string{"foo"}}, "does-not-exist.sh"},
	} {
		t.Run(name, func(t *testing.T) {
			info := &nfpm.Info{Overridables: nfpm.Overridables{RPM: nfpm.RPM{Triggers: []nfpm.RPMTrigger{trigger.trigger}}}}

			_, err := readTriggers(info)
			require.ErrorContains(t, err, trigger.expected)
		})
	}

	t.Run("condition", func(t *testing.T) {
		info := &nfpm.Info{Overridables: nfpm.Overridables{RPM: nfpm.RPM{Triggers: []nfpm.RPMTrigger{{
			Type: "in", Script: "../testdata/scripts/postinstall.sh", Conditions: []string{"foo >>> 2"},
		}}}}}

		_, err := readTriggers(info)
		require.ErrorContains(t, err, "invalid condition")
	})

	t.Run("no conditions", func(t *testing.T) {
		info := &nfpm.Info{Overridables: nfpm.Overridables{RPM: nfpm.RPM{Triggers: []nfpm.RPMTrigger{{
			Type: "in", Script: "../testdata/scripts/postinstall.sh",
		}}}}}

		triggers, err := readTriggers(info)
		require.NoError(t, err)
		require.Empty(t, triggers[0].conditions)
	})
}

func TestRenderSpecChangelog(t *testing.T) {
	t.Run("renders newest first", func(t *testing.T) {
		info := exampleInfo()
		info.Changelog = "../testdata/changelog.yaml"

		out, err := renderSpecChangelog(info)
		require.NoError(t, err)

		require.Contains(t, out, "\n%changelog\n")
		// Both entries and their notes are present.
		require.Contains(t, out, "note 1")
		require.Contains(t, out, "note 2")
		require.Contains(t, out, "note 3")
		require.Contains(t, out, "Carlos A Becker")

		// The 1.1.0 entry (2009-12-08) is newer than the 1.0.0 entry
		// (2009-11-10) and must be rendered first.
		require.Less(t,
			bytes.Index([]byte(out), []byte("1.1.0")),
			bytes.Index([]byte(out), []byte("1.0.0")),
			"newest changelog entry must come first",
		)
	})

	t.Run("no changelog configured", func(t *testing.T) {
		out, err := renderSpecChangelog(exampleInfo())
		require.NoError(t, err)
		require.Empty(t, out)
	})

	t.Run("missing changelog file", func(t *testing.T) {
		info := exampleInfo()
		info.Changelog = "../testdata/does-not-exist.yaml"

		out, err := renderSpecChangelog(info)
		require.Error(t, err)
		require.Empty(t, out)
	})
}

func TestChangelogEntriesEmpty(t *testing.T) {
	// A changelog file that parses to zero entries must produce no entries:
	// emitting empty changelog tags would invalidate the package.
	info := exampleInfo()
	info.Changelog = "../testdata/empty-changelog.yaml"

	entries, err := changelogEntries(info)
	require.NoError(t, err)
	require.Nil(t, entries)

	out, err := renderSpecChangelog(info)
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestChangelogEntriesMissingFile(t *testing.T) {
	info := exampleInfo()
	info.Changelog = "../testdata/does-not-exist.yaml"

	entries, err := changelogEntries(info)
	require.Error(t, err)
	require.Nil(t, entries)
	require.Contains(t, err.Error(), "reading changelog")
}

func TestWritePayloadTar(t *testing.T) {
	mtime := time.Unix(1600000000, 0).UTC()
	info := &nfpm.Info{
		MTime: mtime,
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/usr/bin/fake",
					FileInfo:    &files.ContentFileInfo{Owner: "me", Group: "us", Mode: 0o755, MTime: mtime},
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        files.TypeConfig,
					FileInfo:    &files.ContentFileInfo{Mode: 0o644, MTime: mtime},
				},
				{
					Source:      "/usr/bin/fake",
					Destination: "/usr/bin/fakelink",
					Type:        files.TypeSymlink,
					FileInfo:    &files.ContentFileInfo{Owner: "me", Group: "us", MTime: mtime},
				},
				{
					Destination: "/var/lib/fake",
					Type:        files.TypeDir,
					FileInfo:    &files.ContentFileInfo{Owner: "me", Group: "us", Mode: 0o755, MTime: mtime},
				},
				{
					Destination: "/var/log/fake.log",
					Type:        files.TypeRPMGhost,
					FileInfo:    &files.ContentFileInfo{Mode: 0o644, MTime: mtime},
				},
				{
					Destination: "/usr/share/implicit",
					Type:        files.TypeImplicitDir,
					FileInfo:    &files.ContentFileInfo{Mode: 0o755, MTime: mtime},
				},
				{
					Source:      "../testdata/fake",
					Destination: "/only/for/deb",
					Packager:    "deb",
					FileInfo:    &files.ContentFileInfo{Mode: 0o644, MTime: mtime},
				},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writePayloadTar(&buf, info))

	entries := readTar(t, buf.Bytes())

	// Regular file: present with content, owner/group, and normalized mode.
	reg, ok := entries["usr/bin/fake"]
	require.True(t, ok, "regular file must be present")
	assert.Equal(t, byte(tar.TypeReg), reg.header.Typeflag)
	assert.Equal(t, "me", reg.header.Uname)
	assert.Equal(t, "us", reg.header.Gname)
	assert.Equal(t, int64(0o755), reg.header.Mode)
	assert.NotEmpty(t, reg.body)

	// Config file: owner/group default to root.
	cfg, ok := entries["etc/fake/fake.conf"]
	require.True(t, ok, "config file must be present")
	assert.Equal(t, byte(tar.TypeReg), cfg.header.Typeflag)
	assert.Equal(t, "root", cfg.header.Uname)
	assert.Equal(t, "root", cfg.header.Gname)

	// Symlink: carries the link target, mode 0777.
	link, ok := entries["usr/bin/fakelink"]
	require.True(t, ok, "symlink must be present")
	assert.Equal(t, byte(tar.TypeSymlink), link.header.Typeflag)
	assert.Equal(t, "/usr/bin/fake", link.header.Linkname)
	assert.Equal(t, int64(0o777), link.header.Mode)

	// Directory: trailing slash and dir typeflag.
	dir, ok := entries["var/lib/fake/"]
	require.True(t, ok, "directory must be present with trailing slash")
	assert.Equal(t, byte(tar.TypeDir), dir.header.Typeflag)
	assert.Equal(t, "me", dir.header.Uname)

	// Ghost files, implicit dirs and foreign-packager contents are not shipped.
	_, ok = entries["var/log/fake.log"]
	assert.False(t, ok, "ghost files must not be in the payload")
	_, ok = entries["usr/share/implicit"]
	assert.False(t, ok, "implicit directories must not be in the payload")
	_, ok = entries["only/for/deb"]
	assert.False(t, ok, "foreign-packager contents must not be in the payload")
}

func TestBuildPayloadTarError(t *testing.T) {
	// A failing payload write must clean up the temp file and surface the error.
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/does-not-exist",
					Destination: "/usr/bin/missing",
					FileInfo:    &files.ContentFileInfo{Mode: 0o644},
				},
			},
		},
	}

	path, err := buildPayloadTar(info)
	require.Error(t, err)
	require.Empty(t, path)
}

func TestWritePayloadTarMissingSource(t *testing.T) {
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/does-not-exist",
					Destination: "/usr/bin/missing",
					FileInfo:    &files.ContentFileInfo{Mode: 0o644},
				},
			},
		},
	}

	err := writePayloadTar(io.Discard, info)
	require.Error(t, err)
}

type tarEntry struct {
	header *tar.Header
	body   []byte
}

func readTar(tb testing.TB, data []byte) map[string]tarEntry {
	tb.Helper()

	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(tb, err)
	defer gz.Close()

	entries := map[string]tarEntry{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(tb, err)

		body, err := io.ReadAll(tr)
		require.NoError(tb, err)

		entries[hdr.Name] = tarEntry{header: hdr, body: body}
	}
	return entries
}

func TestNormalizeFileMode(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode fs.FileMode
		want uint
	}{
		{"plain", 0o644, 0o644},
		{"exec", 0o755, 0o755},
		{"setuid go-encoded", 0o755 | fs.ModeSetuid, 0o4755},
		{"setgid go-encoded", 0o755 | fs.ModeSetgid, 0o2755},
		{"sticky go-encoded", 0o755 | fs.ModeSticky, 0o1755},
		{"setuid octal-encoded", fs.FileMode(0o4755), 0o4755},
		{"setgid octal-encoded", fs.FileMode(0o2755), 0o2755},
		{"sticky octal-encoded", fs.FileMode(0o1755), 0o1755},
		{"all special octal-encoded", fs.FileMode(0o7755), 0o7755},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeFileMode(tt.mode))
		})
	}
}

func TestWriteScriptSection(t *testing.T) {
	t.Run("empty body is omitted", func(t *testing.T) {
		var b strings.Builder
		writeScriptSection(&b, "post", "")
		require.Empty(t, b.String())
	})

	t.Run("trailing newline preserved", func(t *testing.T) {
		var b strings.Builder
		writeScriptSection(&b, "post", "echo hi\n")
		require.Equal(t, "\n%post\necho hi\n", b.String())
	})

	t.Run("missing trailing newline is added", func(t *testing.T) {
		var b strings.Builder
		writeScriptSection(&b, "post", "echo hi")
		require.Equal(t, "\n%post\necho hi\n", b.String())
	})
}

func TestSpecFileLine(t *testing.T) {
	const dest = "/some/path"
	fi := func(mode fs.FileMode) *files.ContentFileInfo {
		return &files.ContentFileInfo{Mode: mode}
	}

	for _, tt := range []struct {
		name     string
		content  *files.Content
		contains []string
		empty    bool
	}{
		{
			name:    "implicit dir omitted",
			content: &files.Content{Destination: dest, Type: files.TypeImplicitDir, FileInfo: fi(0o755)},
			empty:   true,
		},
		{
			name:     "symlink",
			content:  &files.Content{Destination: dest, Type: files.TypeSymlink, FileInfo: fi(0)},
			contains: []string{`%attr(-, root, root) "/some/path"`},
		},
		{
			name:     "dir",
			content:  &files.Content{Destination: dest, Type: files.TypeDir, FileInfo: fi(0o755)},
			contains: []string{"%dir %attr(0755, root, root)", dest},
		},
		{
			name:     "config",
			content:  &files.Content{Destination: dest, Type: files.TypeConfig, FileInfo: fi(0o644)},
			contains: []string{"%config %attr(0644, root, root)", dest},
		},
		{
			name:     "config noreplace",
			content:  &files.Content{Destination: dest, Type: files.TypeConfigNoReplace, FileInfo: fi(0o644)},
			contains: []string{"%config(noreplace) %attr"},
		},
		{
			name:     "config missingok",
			content:  &files.Content{Destination: dest, Type: files.TypeConfigMissingOK, FileInfo: fi(0o644)},
			contains: []string{"%config(missingok) %attr"},
		},
		{
			name:     "doc",
			content:  &files.Content{Destination: dest, Type: files.TypeRPMDoc, FileInfo: fi(0o644)},
			contains: []string{"%doc %attr"},
		},
		{
			name:     "license",
			content:  &files.Content{Destination: dest, Type: files.TypeRPMLicense, FileInfo: fi(0o644)},
			contains: []string{"%license %attr"},
		},
		{
			name:     "readme",
			content:  &files.Content{Destination: dest, Type: files.TypeRPMReadme, FileInfo: fi(0o644)},
			contains: []string{"%doc %attr"},
		},
		{
			name:     "ghost with mode",
			content:  &files.Content{Destination: dest, Type: files.TypeRPMGhost, FileInfo: fi(0o600)},
			contains: []string{"%ghost %attr(0600, root, root)"},
		},
		{
			name:     "ghost without mode defaults to 0644",
			content:  &files.Content{Destination: dest, Type: files.TypeRPMGhost, FileInfo: fi(0)},
			contains: []string{"%ghost %attr(0644, root, root)"},
		},
		{
			name:     "regular file",
			content:  &files.Content{Destination: dest, FileInfo: fi(0o644)},
			contains: []string{"%attr(0644, root, root)", dest},
		},
		{
			name:     "custom owner and group",
			content:  &files.Content{Destination: dest, FileInfo: &files.ContentFileInfo{Mode: 0o644, Owner: "me", Group: "us"}},
			contains: []string{"%attr(0644, me, us)"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			line := specFileLine(tt.content)
			if tt.empty {
				require.Empty(t, line)
				return
			}
			for _, want := range tt.contains {
				require.Contains(t, line, want)
			}
		})
	}
}

func TestAddContentsFileTypes(t *testing.T) {
	// addContents only configures the builder; sources are not read until Build,
	// so every RPM-specific content type can be exercised without real files.
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{Source: "src", Destination: "/generic", FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/config", Type: files.TypeConfig, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/confignoreplace", Type: files.TypeConfigNoReplace, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/configmissingok", Type: files.TypeConfigMissingOK, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/doc", Type: files.TypeRPMDoc, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/license", Type: files.TypeRPMLicense, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/licence", Type: files.TypeRPMLicence, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "src", Destination: "/readme", Type: files.TypeRPMReadme, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Destination: "/ghost", Type: files.TypeRPMGhost, FileInfo: &files.ContentFileInfo{Mode: 0o644}},
				{Source: "/target", Destination: "/symlink", Type: files.TypeSymlink, FileInfo: &files.ContentFileInfo{}},
				{Destination: "/dir", Type: files.TypeDir, FileInfo: &files.ContentFileInfo{Mode: 0o755}},
				{Destination: "/implicit", Type: files.TypeImplicitDir, FileInfo: &files.ContentFileInfo{Mode: 0o755}},
				{Source: "src", Destination: "/only/for/deb", Packager: "deb", FileInfo: &files.ContentFileInfo{Mode: 0o644}},
			},
		},
	}

	// Should not panic and should walk every switch arm.
	require.NotPanics(t, func() { addContents(rpm.NewPackage(), info) })
}

func TestGenerateSpecPostRequiresAndForeignContents(t *testing.T) {
	info := nfpm.WithDefaults(&nfpm.Info{
		Name:        "spectest",
		Arch:        "amd64",
		Version:     "1.0.0",
		Description: "desc",
		Maintainer:  "maintainer",
		Overridables: nfpm.Overridables{
			RPM: nfpm.RPM{
				Requires: nfpm.RPMRequires{
					Post: []string{"systemd"},
				},
			},
			Contents: []*files.Content{
				{Source: "../testdata/fake", Destination: "/usr/bin/fake", FileInfo: &files.ContentFileInfo{Mode: 0o755}},
				// A foreign-packager entry must be skipped in the %files section.
				{Source: "../testdata/fake", Destination: "/only/for/deb", Packager: "deb", FileInfo: &files.ContentFileInfo{Mode: 0o644}},
			},
		},
	})
	info = setDefaults(info)

	spec, err := generateSpec(info, "spectest-1.0.0.tar.gz")
	require.NoError(t, err)

	require.Contains(t, spec, "Requires(post): systemd")
	require.Contains(t, spec, "/usr/bin/fake")
	require.NotContains(t, spec, "/only/for/deb")
}

func TestSRPMSignature(t *testing.T) {
	info := exampleInfo()
	info.RPM.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.RPM.Signature.KeyPassphrase = "hunter2"

	var buf bytes.Buffer
	require.NoError(t, DefaultSRPM.Package(info, &buf))

	// The source package is produced and carries a non-empty payload.
	require.NotEmpty(t, buf.Bytes())
}

func TestPackageErrors(t *testing.T) {
	for _, pkgr := range []struct {
		name     string
		packager *RPM
	}{
		{"rpm", DefaultRPM},
		{"srpm", DefaultSRPM},
	} {
		t.Run(pkgr.name, func(t *testing.T) {
			for _, tt := range []struct {
				name     string
				mutate   func(*nfpm.Info)
				skipSRPM bool // source packages don't exercise this path
			}{
				{
					name:   "invalid epoch",
					mutate: func(i *nfpm.Info) { i.Epoch = "not-a-number" },
				},
				{
					name:   "unsupported compression",
					mutate: func(i *nfpm.Info) { i.RPM.Compression = "bzip2" },
				},
				{
					name:   "missing script file",
					mutate: func(i *nfpm.Info) { i.Scripts.PreInstall = "../testdata/does-not-exist.sh" },
				},
				{
					name:   "missing changelog file",
					mutate: func(i *nfpm.Info) { i.Changelog = "../testdata/does-not-exist.yaml" },
				},
				{
					// The source package writes Requires(post) into the spec
					// verbatim and never parses the relation, so only the binary
					// path validates it.
					name:     "invalid post-require",
					mutate:   func(i *nfpm.Info) { i.RPM.Requires.Post = []string{"pkg >>> 2"} },
					skipSRPM: true,
				},
			} {
				if tt.skipSRPM && pkgr.name == "srpm" {
					continue
				}
				t.Run(tt.name, func(t *testing.T) {
					info := exampleInfo()
					tt.mutate(info)
					require.Error(t, pkgr.packager.Package(info, io.Discard))
				})
			}
		})
	}
}
