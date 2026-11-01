package nfpm

import (
	"os"
	"regexp"
	"strings"
)

// Protect MSI's component-directory tokens and explicit dollar escapes before
// applying normal environment expansion to the remaining text. Expansion is
// single-pass: dollars in an environment value are not expanded a second time.
var msiLiteralPattern = regexp.MustCompile(`\[\$[A-Za-z_][A-Za-z0-9_.]*\]|\$\$`)

func expandMSIValue(value string, mapping func(string) string) string {
	var b strings.Builder
	start := 0
	for _, span := range msiLiteralPattern.FindAllStringIndex(value, -1) {
		b.WriteString(os.Expand(value[start:span[0]], mapping))
		literal := value[span[0]:span[1]]
		if literal == "$$" {
			b.WriteByte('$')
		} else {
			b.WriteString(literal)
		}
		start = span[1]
	}
	b.WriteString(os.Expand(value[start:], mapping))
	return b.String()
}
