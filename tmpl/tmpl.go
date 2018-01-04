// Package tmpl contains templating utilities
package tmpl

import "strings"

// Join joins strings with `, `
func Join(strs []string) string {
	return strings.Trim(strings.Join(strs, ", "), " ")
}
