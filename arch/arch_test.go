package arch

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goreleaser/nfpm"
)

func getPkgInfoField(t *testing.T, pkgInfo, key string) []string {
	var values []string
	for _, line := range strings.Split(pkgInfo, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, " = ", 2)
		assert.Equalf(t, 2, len(fields), "malformed line: %s", line)
		if fields[0] == key {
			values = append(values, fields[1])
		}
	}
	return values
}

func TestPkgInfoVersion(t *testing.T) {
	table := map[string]struct {
		version string
		info    *nfpm.Info
	}{
		"default_release": {
			version: "1.2.3-1",
			info: &nfpm.Info{
				Version: "1.2.3",
			},
		},
		"version_with_epoch": {
			version: "9:4.5.6-1",
			info: &nfpm.Info{
				Epoch:   "9",
				Version: "4.5.6",
			},
		},
		"version_with_release": {
			version: "15.16.0-17",
			info: &nfpm.Info{
				Version: "15.16",
				Release: "17",
			},
		},
	}

	for name, test := range table {
		test := test
		t.Run(name, func(t *testing.T) {
			pkgInfoBytes, err := getPkgInfo(nfpm.WithDefaults(test.info), 0)
			assert.NoError(t, err)

			versions := getPkgInfoField(t, string(pkgInfoBytes), "pkgver")
			assert.Equalf(t, 1, len(versions), "not exactly one version: %s", versions)
			assert.Equal(t, test.version, versions[0])
		})
	}
}
