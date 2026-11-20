package deb

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/internal/sign"
	"github.com/stretchr/testify/require"
)

func fakeTime() time.Time {
	t, _ := time.Parse(time.RFC1123Z, "Mon, 30 Jan 2023 08:36:31 +0300")
	return t
}

func metaInfo(t *testing.T) *nfpm.Info {
	t.Helper()

	now = fakeTime
	t.Cleanup(func() { now = time.Now })

	info := exampleInfo()
	info.EnableMetadata = true
	info.Deb.Metadata = nfpm.DebMetadata{
		Distribution: "unstable",
		Urgency:      "medium",
	}
	info.Target = Default.ConventionalFileName(info)
	return info
}

func packageMetadata(t *testing.T, info *nfpm.Info) (string, error) {
	t.Helper()

	var w bytes.Buffer
	err := Default.PackageMetadata(&nfpm.MetaInfo{
		Info:    info,
		Package: bytes.NewReader([]byte{1, 2, 3}),
	}, &w)
	return w.String(), err
}

func TestMetaFilename(t *testing.T) {
	for name, tc := range map[string]struct {
		target   string
		expected string
	}{
		"empty target":     {"", "foo_1.0.0_amd64.changes"},
		"deb extension":    {"bar_1.0.0_amd64.deb", "bar_1.0.0_amd64.changes"},
		"no extension":     {"artifact", "artifact.changes"},
		"other extension":  {"artifact.pkg", "artifact.pkg.changes"},
		"deb in directory": {"out.deb/foo_1.0.0_amd64.deb", "out.deb/foo_1.0.0_amd64.changes"},
		"deb in directory, no extension": {
			"out.deb/artifact", "out.deb/artifact.changes",
		},
	} {
		t.Run(name, func(t *testing.T) {
			info := exampleInfo()
			info.Target = tc.target

			got := Default.ConventionalMetadataFileName(info)
			require.Equal(t, tc.expected, got)
			require.NotEqual(t, tc.target, got)
		})
	}
}

func TestMetaFields(t *testing.T) {
	info := metaInfo(t)
	info.Deb.Metadata.ChangedBy = "Carlos A Becker <pkg@carlosbecker.com>"
	info.Deb.Metadata.Fields = map[string]string{
		"Bugs":  "https://github.com/goreleaser/nfpm/issues",
		"Empty": "",
	}

	got, err := packageMetadata(t, info)
	require.NoError(t, err)

	golden := "testdata/changes.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(got), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), got)
}

func TestMetaDescription(t *testing.T) {
	info := metaInfo(t)
	info.Description = "Foo does things\n\nAnd a longer description\nwith more lines."

	got, err := packageMetadata(t, info)
	require.NoError(t, err)
	require.Contains(t, got, "\nDescription:\n foo - Foo does things\n")
	require.NotContains(t, got, "longer description")
}

func TestMetaVersion(t *testing.T) {
	info := metaInfo(t)
	info.Epoch = "2"
	info.Prerelease = "rc1"
	info.VersionMetadata = "git"
	info.Release = "3"
	info.Target = Default.ConventionalFileName(info)

	got, err := packageMetadata(t, info)
	require.NoError(t, err)
	require.Contains(t, got, "\nVersion: 2:1.0.0~rc1+git-3\n")
	require.Contains(t, got, "\n foo (2:1.0.0~rc1+git-3) unstable; urgency=medium\n")
}

func TestMetaFilesPlaceholders(t *testing.T) {
	info := metaInfo(t)
	info.Section = ""
	info.Priority = ""

	got, err := packageMetadata(t, info)
	require.NoError(t, err)

	_, files, ok := strings.Cut(got, "\nFiles:\n")
	require.True(t, ok)
	row := strings.TrimSpace(strings.SplitN(files, "\n", 2)[0])
	require.Len(t, strings.Fields(row), 5)
	require.Equal(t, "5289df737df57326fcdd22597afb1fac 3 - - foo_1.0.0_amd64.deb", row)
}

func TestMetaRequiredFields(t *testing.T) {
	for name, tc := range map[string]struct {
		modify func(*nfpm.Info)
		err    string
	}{
		"distribution": {
			func(i *nfpm.Info) { i.Deb.Metadata.Distribution = "" },
			"deb.metadata.distribution is required",
		},
		"urgency": {
			func(i *nfpm.Info) { i.Deb.Metadata.Urgency = "" },
			"deb.metadata.urgency is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			info := metaInfo(t)
			tc.modify(info)

			got, err := packageMetadata(t, info)
			require.ErrorContains(t, err, tc.err)
			require.Empty(t, got)
		})
	}
}

func TestMetaTargetFileName(t *testing.T) {
	t.Run("custom base name", func(t *testing.T) {
		info := metaInfo(t)
		info.Target = "foo.deb"

		got, err := packageMetadata(t, info)
		require.ErrorContains(t, err, `"foo.deb" is not supported`)
		require.ErrorContains(t, err, `use "foo_1.0.0_amd64.deb"`)
		require.Empty(t, got)
	})

	t.Run("conventional name in a directory", func(t *testing.T) {
		info := metaInfo(t)
		info.Target = "out.deb/" + Default.ConventionalFileName(info)

		got, err := packageMetadata(t, info)
		require.NoError(t, err)
		require.Contains(t, got, " foo_1.0.0_amd64.deb\n")
		require.NotContains(t, got, "out.deb/")
	})

	t.Run("empty target", func(t *testing.T) {
		info := metaInfo(t)
		info.Target = ""

		got, err := packageMetadata(t, info)
		require.NoError(t, err)
		require.Contains(t, got, " foo_1.0.0_amd64.deb\n")
	})
}

func TestMetaSignature(t *testing.T) {
	info := metaInfo(t)
	info.Deb.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.Deb.Signature.KeyPassphrase = "hunter2"

	got, err := packageMetadata(t, info)
	require.NoError(t, err)

	_, err = sign.PGPReadMessage([]byte(got), "../internal/sign/testdata/pubkey.asc")
	require.NoError(t, err)
}

func TestMetaSignatureError(t *testing.T) {
	info := metaInfo(t)
	info.Deb.Signature.KeyFile = "/does/not/exist"

	_, err := packageMetadata(t, info)

	var expectedError *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &expectedError)
}

func TestMetaSignatureCallback(t *testing.T) {
	clearSign := func(r io.Reader) ([]byte, error) {
		return sign.PGPClearSignWithKeyID(r, "../internal/sign/testdata/privkey.asc", "hunter2", nil)
	}

	t.Run("dpkg-sig", func(t *testing.T) {
		info := metaInfo(t)
		info.Deb.Signature.Method = "dpkg-sig"
		info.Deb.Signature.SignFn = clearSign

		got, err := packageMetadata(t, info)
		require.NoError(t, err)

		msg, err := sign.PGPReadMessage([]byte(got), "../internal/sign/testdata/pubkey.asc")
		require.NoError(t, err)
		require.Contains(t, string(msg), "Format: 1.8")
	})

	t.Run("takes priority over key file", func(t *testing.T) {
		info := metaInfo(t)
		info.Deb.Signature.Method = "dpkg-sig"
		info.Deb.Signature.KeyFile = "/does/not/exist"
		info.Deb.Signature.SignFn = clearSign

		_, err := packageMetadata(t, info)
		require.NoError(t, err)
	})

	t.Run("callback error", func(t *testing.T) {
		info := metaInfo(t)
		info.Deb.Signature.Method = "dpkg-sig"
		info.Deb.Signature.SignFn = func(io.Reader) ([]byte, error) {
			return nil, errors.New("boom")
		}

		_, err := packageMetadata(t, info)
		var expectedError *nfpm.ErrSigningFailure
		require.ErrorAs(t, err, &expectedError)
		require.ErrorContains(t, err, "boom")
	})

	t.Run("debsign callback is not reused", func(t *testing.T) {
		called := false
		info := metaInfo(t)
		info.Deb.Signature.SignFn = func(io.Reader) ([]byte, error) {
			called = true
			return nil, nil
		}

		got, err := packageMetadata(t, info)
		var expectedError *nfpm.ErrSigningFailure
		require.ErrorAs(t, err, &expectedError)
		require.ErrorIs(t, err, errUnsupportedChangesSignFn)
		require.Empty(t, got)
		require.False(t, called)
	})
}
