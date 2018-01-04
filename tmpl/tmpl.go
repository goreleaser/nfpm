package tmpl

import "strings"

func Join(strs []string) string {
	return strings.Trim(strings.Join(strs, ", "), " ")
}
