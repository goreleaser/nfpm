package cmd

import (
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"
)

func TestSchemaDebHasNoRequiredFields(t *testing.T) {
	schema := jsonschema.Reflect(&nfpm.Config{})

	for _, name := range []string{"Deb", "DebMetadata"} {
		def, ok := schema.Definitions[name]
		require.True(t, ok, name)
		require.Empty(t, def.Required, name)
	}
}
