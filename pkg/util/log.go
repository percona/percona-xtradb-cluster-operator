package util

import (
	"os"
	"strings"
)

func IsLogLevelVerbose() bool {
	l, found := os.LookupEnv("LOG_LEVEL")
	if !found {
		return false
	}

	return strings.ToUpper(l) == "VERBOSE"
}
