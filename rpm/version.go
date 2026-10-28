package rpm

import (
	"strings"

	"go.digitalxero.dev/rpm"
)

type evr struct {
	epoch   string
	version string
	release string
}

// parseEVR splits an [epoch:]version[-release] string like RPM's parseEVR.
func parseEVR(s string) evr {
	var v evr
	if i := segmentEnd(s, isDigit); i < len(s) && s[i] == ':' {
		v.epoch, s = s[:i], s[i+1:]
		if v.epoch == "" {
			v.epoch = "0"
		}
	}
	if i := strings.LastIndexByte(s, '-'); i >= 0 {
		s, v.release = s[:i], s[i+1:]
	}
	v.version = s
	return v
}

// conditionsOverlap reports whether a package can match both conditions,
// following rpmdsCompare in RPM.
func conditionsOverlap(a, b rpm.Relation) bool {
	if a.Name() != b.Name() {
		return false
	}

	const senses = rpm.SenseLess | rpm.SenseGreater | rpm.SenseEqual
	aSense, bSense := a.Sense()&senses, b.Sense()&senses
	if aSense == 0 || bSense == 0 || a.Version() == "" || b.Version() == "" {
		return true
	}

	av, bv := parseEVR(a.Version()), parseEVR(b.Version())
	cmp := 0
	switch {
	case av.epoch != "" && bv.epoch != "":
		cmp = rpmvercmp(av.epoch, bv.epoch)
	case strings.TrimLeft(av.epoch, "0") != "":
		cmp = 1
	case strings.TrimLeft(bv.epoch, "0") != "":
		cmp = -1
	}
	if cmp == 0 {
		cmp = rpmvercmp(av.version, bv.version)
	}
	if cmp == 0 {
		switch {
		case av.release != "" && bv.release != "":
			cmp = rpmvercmp(av.release, bv.release)
		case av.release != "" && bSense&rpm.SenseEqual != 0,
			bv.release != "" && aSense&rpm.SenseEqual != 0:
			return true
		}
	}

	switch {
	case cmp < 0:
		return aSense&rpm.SenseGreater != 0 || bSense&rpm.SenseLess != 0
	case cmp > 0:
		return aSense&rpm.SenseLess != 0 || bSense&rpm.SenseGreater != 0
	default:
		return aSense&bSense != 0
	}
}

// rpmvercmp compares two version strings like RPM's rpmvercmp, and returns
// -1, 0 or 1.
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}

	for a != "" || b != "" {
		a, b = trimVersionSeparators(a), trimVersionSeparators(b)

		if strings.HasPrefix(a, "~") || strings.HasPrefix(b, "~") {
			if !strings.HasPrefix(a, "~") {
				return 1
			}
			if !strings.HasPrefix(b, "~") {
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}

		if strings.HasPrefix(a, "^") || strings.HasPrefix(b, "^") {
			switch {
			case a == "":
				return -1
			case b == "":
				return 1
			case !strings.HasPrefix(a, "^"):
				return 1
			case !strings.HasPrefix(b, "^"):
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}

		if a == "" || b == "" {
			break
		}

		inSegment := isAlpha
		if isDigit(a[0]) {
			inSegment = isDigit
		}
		i, j := segmentEnd(a, inSegment), segmentEnd(b, inSegment)
		one, two := a[:i], b[:j]
		a, b = a[i:], b[j:]

		if two == "" {
			if isDigit(one[0]) {
				return 1
			}
			return -1
		}

		if isDigit(one[0]) {
			one, two = strings.TrimLeft(one, "0"), strings.TrimLeft(two, "0")
			if len(one) != len(two) {
				if len(one) > len(two) {
					return 1
				}
				return -1
			}
		}

		if cmp := strings.Compare(one, two); cmp != 0 {
			return cmp
		}
	}

	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

func trimVersionSeparators(s string) string {
	for s != "" && !isAlpha(s[0]) && !isDigit(s[0]) && s[0] != '~' && s[0] != '^' {
		s = s[1:]
	}
	return s
}

func segmentEnd(s string, inSegment func(byte) bool) int {
	i := 0
	for i < len(s) && inSegment(s[i]) {
		i++
	}
	return i
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isAlpha(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
