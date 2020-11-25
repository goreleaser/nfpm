package files_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/goreleaser/nfpm/files"
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
  file_info:
    mode: 0644
    packager: "rpm"
    mtime: 2008-01-02T15:04:05Z
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	assert.Len(t, config.Contents, 2)
	for _, f := range config.Contents {
		fmt.Printf("%+#v\n", f)
		assert.Equal(t, f.Source, "a")
		assert.Equal(t, f.Destination, "b")
	}
}

func TestMapperDecode(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
  a: b
  a2: b2
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	assert.Len(t, config.Contents, 2)
	for _, f := range config.Contents {
		fmt.Printf("%+#v\n", f)
		assert.Equal(t, f.Packager, "")
		assert.Equal(t, f.Type, "")
	}
}

func TestStringDecode(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents: /path/to/a/tgz
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.Error(t, err)
	assert.Equal(t, err.Error(), "not implemented")
}
