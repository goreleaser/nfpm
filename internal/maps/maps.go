package maps

import (
	"sort"

	"golang.org/x/exp/maps"
)

func Keys[T any](m map[string]T) []string {
	keys := maps.Keys(m)
	sort.Strings(keys)
	return keys
}
