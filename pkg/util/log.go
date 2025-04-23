package util

import (
	"os"
	"strconv"
	"strings"
)

func IsLogLevelVerbose() bool {
	l, found := os.LookupEnv("LOG_LEVEL")
	if !found {
		return false
	}

	return strings.ToUpper(l) == "VERBOSE"
}

func IsLogStructured() bool {
	s, found := os.LookupEnv("LOG_STRUCTURED")
	if !found {
		return false
	}

	useJson, err := strconv.ParseBool(s)
	if err != nil {
		return false
	}

	return useJson
}
