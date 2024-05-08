package ipk

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/goreleaser/nfpm/v2/files"
)

// newTGZ creates a new tar.gz archive with the given name and populates it
// with the given function.
//
// The function returns the bytes of the archive, its size and an error if any.
func newTGZ(name string, populate func(*tar.Writer) error) ([]byte, error) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tarball := tar.NewWriter(gz)

	// the writers are properly closed later, this is just in case that we error out
	defer gz.Close()      // nolint: errcheck
	defer tarball.Close() // nolint: errcheck

	if err := populate(tarball); err != nil {
		return nil, fmt.Errorf("cannot populate '%s': %w", name, err)
	}

	if err := tarball.Close(); err != nil {
		return nil, fmt.Errorf("cannot close '%s': %w", name, err)
	}

	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("cannot close '%s': %w", name, err)
	}

	return buf.Bytes(), nil
}

// writeFile writes a file from the filesystem to the tarball.
func writeFile(out *tar.Writer, file *files.Content) (int64, error) {
	f, err := os.OpenFile(file.Source, os.O_RDONLY, 0o600) //nolint:gosec
	if err != nil {
		return 0, fmt.Errorf("could not open file %s to read and include in the archive: %w", file.Source, err)
	}
	defer f.Close() // nolint: errcheck

	header, err := tar.FileInfoHeader(file, file.Source)
	if err != nil {
		return 0, err
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}

	size := int64(len(content))

	// tar.FileInfoHeader only uses file.Mode().Perm() which masks the mode with
	// 0o777 which we don't want because we want to be able to set the suid bit.
	header.Mode = int64(file.Mode())
	header.Format = tar.FormatGNU
	header.Name = files.AsExplicitRelativePath(file.Destination)
	header.Size = size
	header.Uname = file.FileInfo.Owner
	header.Gname = file.FileInfo.Group
	if err := out.WriteHeader(header); err != nil {
		return 0, fmt.Errorf("cannot write tar header for file %s to archive: %w", file.Source, err)
	}

	n, err := out.Write(content)
	if err != nil {
		return 0, fmt.Errorf("%s: failed to copy: %w", file.Source, err)
	}

	if int64(n) != size {
		return 0, fmt.Errorf("%s: failed to copy: expected %d bytes, copied %d", file.Source, size, n)
	}

	return size, nil
}

// writeToFile writes a file to the tarball where the contents are an array of bytes.
func writeToFile(out *tar.Writer, filename string, content []byte, mtime time.Time) error {
	header := tar.Header{
		Name:     files.AsExplicitRelativePath(filename),
		Size:     int64(len(content)),
		Mode:     0o644,
		ModTime:  mtime,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatGNU,
	}

	if err := out.WriteHeader(&header); err != nil {
		return fmt.Errorf("cannot write file header %s to archive: %w", header.Name, err)
	}

	_, err := out.Write(content)
	if err != nil {
		return fmt.Errorf("cannot write file %s payload: %w", header.Name, err)
	}
	return nil
}
