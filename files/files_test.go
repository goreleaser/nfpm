package files_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/goreleaser/nfpm/v2/files"
)

type testStruct struct {
	Contents files.Contents `yaml:"contents"`
}

func TestBasicDecode(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: a
  dst: b
- src: a
  dst: b
  type: "config|noreplace"
  packager: "rpm"
  file_info:
    mode: 0644
    mtime: 2008-01-02T15:04:05Z
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	assert.Len(t, config.Contents, 2)
	for _, f := range config.Contents {
		t.Logf("%+#v\n", f)
		assert.Equal(t, f.Source, "a")
		assert.Equal(t, f.Destination, "b")
	}
}

func TestRace(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: a
  dst: b
  type: symlink
`))
	err := dec.Decode(&config)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := files.ExpandContentGlobs(config.Contents, false)
			require.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestCollision(t *testing.T) {
	configuredFiles := []*files.Content{
		{Source: "../testdata/whatever.conf", Destination: "/samedestination"},
		{Source: "../testdata/whatever2.conf", Destination: "/samedestination"},
	}

	_, err := files.ExpandContentGlobs(configuredFiles, true)
	require.ErrorIs(t, err, files.ErrContentCollision)
}
