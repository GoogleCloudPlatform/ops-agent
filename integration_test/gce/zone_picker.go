package gce

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/smallnest/weighted"
)

// weightedRoundRobin implements a thread safe weighted round robin algorithm.
// According to the `weighted` library, it is the same algorithm as the one
// used by nginx to do weighted round robin.
type weightedRoundRobin struct {
	mutex sync.Mutex
	// This type implements the weighted round robin algorithm. I don't know what
	// SW stands for, maybe "smooth weighted".
	sw *weighted.SW
}

// Next() selects the next zone to spawn a VM in.
// It is thread safe.
func (wrr *weightedRoundRobin) Next() string {
	wrr.mutex.Lock()
	defer wrr.mutex.Unlock()

	return wrr.sw.Next().(string)
}

// parseZoneEnvironmentVariables looks at `weightedZones` (and `zone` if
// needed) and extracts the zones and weights into a a weighted round robin
// zone picker.
// TODO(martijnvans): Remove support for `zone`.
func newZonePicker(weightedZones, zone string) (*weightedRoundRobin, error) {
	if weightedZones == "" {
		if zone == "" {
			return nil, errors.New("either ZONE or WEIGHTED_ZONES must be specified")
		}
		weightedZones = zone + "=1"
	}

	sw := &weighted.SW{}
	// Each zoneSpec should look like <string>=<integer>.
	for _, zoneSpec := range strings.Split(weightedZones, ",") {
		zoneAndWeight := strings.Split(zoneSpec, "=")
		if len(zoneAndWeight) != 2 {
			return nil, fmt.Errorf(`invalid zone specification %q from WEIGHTED_ZONES=%q; should be like "us-central1=5"`, zoneSpec, weightedZones)
		}
		weight, err := strconv.Atoi(zoneAndWeight[1])
		if err != nil {
			return nil, fmt.Errorf("Zone specification %q had non-integer weight %q", zoneSpec, zoneAndWeight[1])
		}
		sw.Add(zoneAndWeight[0], weight)
	}
	return &weightedRoundRobin{
		mutex: sync.Mutex{},
		sw:    sw,
	}, nil
}
