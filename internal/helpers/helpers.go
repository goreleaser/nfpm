package helpers

import "strconv"

func IsInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
