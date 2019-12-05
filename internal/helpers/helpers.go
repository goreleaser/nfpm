// package for common helpers
package helpers

import "strconv"

// returns true if s is int, false otherwise
func IsInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
