package rpm

import (
	"compress/gzip"
	"fmt"
	"strconv"
	"strings"

	"go.digitalxero.dev/rpm"
)

// parseCompression maps the nfpm compression setting (an algorithm name with an
// optional ":level" suffix, e.g. "zstd:19") onto a rpm.Compressor. xz and lzma
// do not take a level, so any provided level is ignored for them.
func parseCompression(compression string) (rpm.Compressor, error) {
	name, levelStr, hasLevel := strings.Cut(compression, ":")

	level := 0
	levelSet := false
	if hasLevel && levelStr != "" {
		l, err := strconv.Atoi(levelStr)
		if err != nil {
			return nil, fmt.Errorf("rpm: invalid compression level %q: %w", levelStr, err)
		}
		level, levelSet = l, true
	}

	switch name {
	case "", "gzip", "gz":
		if levelSet {
			return rpm.GzipCompressor(level), nil
		}
		return rpm.GzipCompressor(gzip.DefaultCompression), nil
	case "zstd":
		if levelSet {
			return rpm.ZstdCompressor(level), nil
		}
		return rpm.ZstdCompressor(3), nil
	case "xz":
		return rpm.XzCompressor(), nil
	case "lzma":
		return rpm.LzmaCompressor(), nil
	default:
		return nil, fmt.Errorf("rpm: unsupported compression %q", name)
	}
}
