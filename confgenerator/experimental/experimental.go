package experimental

import (
	"fmt"
	"os"
	"strings"
)

var (
	PrometheusReceiver bool

	featureMap = map[string]*bool{
		"prometheus_receiver": &PrometheusReceiver,
	}
)

func Load() error {
	envVar := os.Getenv("EXPERIMENTAL_FEATURES")
	enabledList := strings.Split(envVar, ",")
	alreadyLoaded := map[string]bool{}

	// strings.Split is weird about splitting empty strings;
	// we want an empty list if envVar is empty
	if len(enabledList) == 1 && len(enabledList[0]) == 0 {
		enabledList = []string{}
	}

	// Validate first
	for _, e := range enabledList {
		e := strings.TrimSpace(e)
		if featureMap[e] == nil {
			return fmt.Errorf("unrecognized experimental feature: %v", e)
		}
		if alreadyLoaded[e] {
			return fmt.Errorf("duplicate experimental feature: %v", e)
		}
		alreadyLoaded[e] = true
	}

	// List is valid; reset to default state and then process the list
	for _, p := range featureMap {
		*p = false
	}
	for _, e := range enabledList {
		e := strings.TrimSpace(e)
		*featureMap[e] = true
	}
	return nil
}
