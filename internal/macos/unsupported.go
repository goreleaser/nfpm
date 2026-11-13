package macos

import (
	"log"

	"github.com/goreleaser/nfpm/v2"
)

// WarnRelationships reports the package relationships that are set, none of
// which a macOS format can express. They are warnings, not errors, so one
// configuration can keep serving the formats that do support them.
func WarnRelationships(packager string, info *nfpm.Info) {
	for _, relationship := range []struct {
		name   string
		values []string
	}{
		{"depends", info.Depends},
		{"recommends", info.Recommends},
		{"suggests", info.Suggests},
		{"conflicts", info.Conflicts},
		{"replaces", info.Replaces},
		{"provides", info.Provides},
	} {
		if len(relationship.values) > 0 {
			log.Printf(
				"warning: %s: macOS has no package relationships, %s is ignored",
				packager, relationship.name)
		}
	}
}

// WarnScripts reports the given scripts that are set but that the format
// never runs. names and scripts are parallel.
func WarnScripts(packager, reason string, names, scripts []string) {
	for n, script := range scripts {
		if script != "" {
			log.Printf("warning: %s: %s, the %s script is ignored", packager, reason, names[n])
		}
	}
}
