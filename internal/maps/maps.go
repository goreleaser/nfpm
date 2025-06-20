package maps

import (
	"maps"
	"slices"
)

func Keys[T any](m map[string]T) []string {
	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)
	return keys
}
