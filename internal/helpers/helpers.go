// Package helpers provides common helper methods
package helpers

import "strconv"

// IsInt returns true if the given string is an int.
func IsInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
