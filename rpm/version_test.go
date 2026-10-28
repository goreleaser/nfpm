package rpm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.digitalxero.dev/rpm"
)

func TestRPMVercmp(t *testing.T) {
	for _, tc := range []struct {
		a, b     string
		expected int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "2.0", -1},
		{"2.0.1", "2.0", 1},
		{"2.0.1a", "2.0.1", 1},
		{"5.5p1", "5.5p10", -1},
		{"10xyz", "10.1xyz", -1},
		{"xyz.4", "8", -1},
		{"6.0.rc1", "6.0", 1},
		{"10b2", "10a1", 1},
		{"1.0a", "1.0aa", -1},
		{"10.0001", "10.1", 0},
		{"4.999.9", "5.0", -1},
		{"2_0", "2.0", 0},
		{"+", "_", 0},
		{"1.0~rc1", "1.0", -1},
		{"1.0~rc1~git123", "1.0~rc1", -1},
		{"1.0^", "1.0", 1},
		{"1.0^git1", "1.01", -1},
		{"1.0^20160101", "1.0.1", -1},
		{"1.0^20160102", "1.0^20160101^git1", 1},
		{"1.0~rc1^git1", "1.0~rc1", 1},
		{"1.0^git1~pre", "1.0^git1", -1},
	} {
		require.Equal(t, tc.expected, rpmvercmp(tc.a, tc.b), "%s <=> %s", tc.a, tc.b)
		require.Equal(t, -tc.expected, rpmvercmp(tc.b, tc.a), "%s <=> %s", tc.b, tc.a)
	}
}

func TestParseEVR(t *testing.T) {
	require.Equal(t, evr{version: "1.0"}, parseEVR("1.0"))
	require.Equal(t, evr{epoch: "2", version: "1.0", release: "3.el9"}, parseEVR("2:1.0-3.el9"))
	require.Equal(t, evr{epoch: "0", version: "1.0"}, parseEVR(":1.0"))
	require.Equal(t, evr{version: "1.0-rc1", release: "2"}, parseEVR("1.0-rc1-2"))
}

func TestConditionsOverlap(t *testing.T) {
	for _, tc := range []struct {
		a, b     string
		expected bool
	}{
		{"foo", "bar", false},
		{"foo", "foo < 2.0", true},
		{"foo < 2.0", "foo >= 2.0", false},
		{"foo <= 2.0", "foo >= 2.0", true},
		{"foo < 2.0", "foo > 1.0", true},
		{"foo = 1.0", "foo = 2.0", false},
		{"foo = 1.0", "foo = 1.0-3", true},
		{"foo < 1.0-3", "foo = 1.0", true},
		{"foo < 1.0-3", "foo >= 1.0-3", false},
		{"foo < 2:1.0", "foo >= 1:3.0", true},
		{"foo < 1:1.0", "foo >= 3.0", true},
		{"foo = 0:1.0", "foo = 1.0", true},
		{"foo < 2.0~rc1", "foo >= 2.0", false},
	} {
		a, err := rpm.ParseRelation(tc.a)
		require.NoError(t, err)
		b, err := rpm.ParseRelation(tc.b)
		require.NoError(t, err)
		require.Equal(t, tc.expected, conditionsOverlap(a, b), "%s / %s", tc.a, tc.b)
		require.Equal(t, tc.expected, conditionsOverlap(b, a), "%s / %s", tc.b, tc.a)
	}
}
