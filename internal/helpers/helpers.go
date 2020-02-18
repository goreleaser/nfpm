// Package helpers provides common helper methods
package helpers

import "strconv"

// IsInt returns true if s is int, false otherwise
func IsInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
