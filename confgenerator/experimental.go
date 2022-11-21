package confgenerator

import (
	"os"
	"strings"
)

func IsExperimentalFeatureEnabled(receiver string) bool {
	enabledList := strings.Split(os.Getenv("EXPERIMENTAL_FEATURES"), ",")
	for _, e := range enabledList {
		if e == receiver {
			return true
		}
	}
	return false
}
