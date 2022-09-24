package rpmpack

import (
	"io"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/klauspost/compress/zstd"
	gzip "github.com/klauspost/pgzip"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

func TestFileOwner(t *testing.T) {
	r, err := NewRPM(RPMMetaData{})
	if err != nil {
		t.Fatalf("NewRPM returned error %v", err)
	}
	group := "testGroup"
	user := "testUser"

	r.AddFile(RPMFile{
		Name:  "/usr/local/hello",
		Body:  []byte("content of the file"),
		Group: group,
		Owner: user,
	})

	if err := r.Write(io.Discard); err != nil {
		t.Errorf("NewRPM returned error %v", err)
	}
	if r.fileowners[0] != user {
		t.Errorf("File owner shoud be %s but is %s", user, r.fileowners[0])
	}
	if r.filegroups[0] != group {
		t.Errorf("File owner shoud be %s but is %s", group, r.filegroups[0])
	}
}

// https://github.com/google/rpmpack/issues/49
func Test100644(t *testing.T) {
	r, err := NewRPM(RPMMetaData{})
	if err != nil {
		t.Fatalf("NewRPM returned error %v", err)
	}
	r.AddFile(RPMFile{
		Name: "/usr/local/hello",
		Body: []byte("content of the file"),
		Mode: 0o100644,
	})

	if err := r.Write(io.Discard); err != nil {
		t.Errorf("Write returned error %v", err)
	}
	if r.filemodes[0] != 0o100644 {
		t.Errorf("file mode want 0100644, got %o", r.filemodes[0])
	}
	if r.filelinktos[0] != "" {
		t.Errorf("linktos want empty (not a symlink), got %q", r.filelinktos[0])
	}
}

func TestCompression(t *testing.T) {
	testCases := []struct {
		Type           string
		Compressors    []string
		ExpectedWriter io.Writer
	}{
		{
			Type: "gzip",
			Compressors: []string{
				"", "gzip", "gzip:1", "gzip:2", "gzip:3",
				"gzip:4", "gzip:5", "gzip:6", "gzip:7", "gzip:8", "gzip:9",
			},
			ExpectedWriter: &gzip.Writer{},
		},
		{
			Type:           "gzip",
			Compressors:    []string{"gzip:fast", "gzip:10"},
			ExpectedWriter: nil, // gzip requires an integer level from -2 to 9
		},
		{
			Type:           "lzma",
			Compressors:    []string{"lzma"},
			ExpectedWriter: &lzma.Writer{},
		},
		{
			Type:           "lzma",
			Compressors:    []string{"lzma:fast", "lzma:1"},
			ExpectedWriter: nil, // lzma does not support specifying the compression level
		},
		{
			Type:           "xz",
			Compressors:    []string{"xz"},
			ExpectedWriter: &xz.Writer{},
		},
		{
			Type:           "xz",
			Compressors:    []string{"xz:fast", "xz:1"},
			ExpectedWriter: nil, // xz does not support specifying the compression level
		},
		{
			Type: "zstd",
			Compressors: []string{
				"zstd", "zstd:fastest", "zstd:default", "zstd:better",
				"zstd:best", "zstd:BeSt", "zstd:0", "zstd:4", "zstd:8", "zstd:15",
			},
			ExpectedWriter: &zstd.Encoder{},
		},
		{
			Type:           "zstd",
			Compressors:    []string{"xz:worst"},
			ExpectedWriter: nil, // only integers levels or one of the pre-defined string values
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		for _, compressor := range testCase.Compressors {
			t.Run(compressor, func(t *testing.T) {
				r, err := NewRPM(RPMMetaData{
					Compressor: compressor,
				})
				if err != nil {
					if testCase.ExpectedWriter == nil {
						return // an error is expected
					}

					t.Fatalf("NewRPM returned error %v", err)
				}

				if testCase.ExpectedWriter == nil {
					t.Fatalf("compressor %q should have produced an error", compressor)
				}

				if r.RPMMetaData.Compressor != testCase.Type {
					t.Fatalf("expected compressor %q, got %q", compressor,
						r.RPMMetaData.Compressor)
				}

				expectedWriterType := reflect.Indirect(reflect.ValueOf(
					testCase.ExpectedWriter)).String()
				actualWriterType := reflect.Indirect(reflect.ValueOf(
					r.compressedPayload)).String()

				if expectedWriterType != actualWriterType {
					t.Fatalf("expected writer to be %T, got %T instead",
						testCase.ExpectedWriter, r.compressedPayload)
				}
			})
		}
	}
}

func TestAllowListDirs(t *testing.T) {
	r, err := NewRPM(RPMMetaData{})
	if err != nil {
		t.Fatalf("NewRPM returned error %v", err)
	}

	r.AddFile(RPMFile{
		Name: "/usr/local/dir1",
		Mode: 0o40000,
	})
	r.AddFile(RPMFile{
		Name: "/usr/local/dir2",
		Mode: 0o40000,
	})

	r.AllowListDirs(map[string]bool{"/usr/local/dir1": true})

	if err := r.Write(io.Discard); err != nil {
		t.Errorf("NewRPM returned error %v", err)
	}
	expected := map[string]RPMFile{"/usr/local/dir1": {Name: "/usr/local/dir1", Mode: 0o40000}}
	if d := cmp.Diff(expected, r.files); d != "" {
		t.Errorf("Expected dirs differs (want->got):\n%v", d)
	}
}
