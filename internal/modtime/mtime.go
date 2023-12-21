package modtime

import (
	"os"
	"strconv"
	"time"
)

func FromEnv() time.Time {
	epoch := os.Getenv("SOURCE_DATE_EPOCH")
	if epoch == "" {
		return time.Time{}
	}
	sde, err := strconv.ParseInt(epoch, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sde, 0).UTC()
}

func Get(times ...time.Time) time.Time {
	for _, t := range times {
		if !t.IsZero() {
			return t
		}
	}
	return time.Now()
}
