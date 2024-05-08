package ipk

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newTGZ(t *testing.T) {
	unknownErr := errors.New("unknown error")

	tests := []struct {
		description string
		name        string
		populate    func(*tar.Writer) error
		file        string
		expectedErr error
	}{
		{
			description: "simple",
			name:        "simple.tar",
			populate: func(tw *tar.Writer) error {
				return writeToFile(tw, "simple.txt", []byte("hello, world"), time.Now())
			},
			file: "./simple.txt",
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			got, err := newTGZ(tc.name, tc.populate)

			if tc.expectedErr == nil {
				require.NoError(err)
				require.NotNil(got)

				gz, err := gzip.NewReader(bytes.NewReader(got))
				require.NoError(err)
				require.NotNil(gz)
				defer gz.Close() // nolint: errcheck

				assert.True(tarContains(t, gz, tc.file))
				return
			}

			require.Error(err)
			if !errors.Is(tc.expectedErr, unknownErr) {
				assert.ErrorIs(err, tc.expectedErr)
			}
		})
	}
}

func extractFileFromTar(tb testing.TB, tarFile []byte, filename string) []byte {
	tb.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) != path.Join("/", filename) {
			continue
		}

		fileContents, err := io.ReadAll(tr)
		require.NoError(tb, err)

		return fileContents
	}

	tb.Fatalf("file %q does not exist in tar", filename)

	return nil
}

func tarContains(tb testing.TB, r io.Reader, filename string) bool {
	tb.Helper()

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) == path.Join("/", filename) { // nolint:gosec
			return true
		}
	}

	return false
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
