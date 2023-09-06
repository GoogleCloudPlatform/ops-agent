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

// newZonePicker looks at `zones` and extracts the zones and weights into a a
// weighted round robin zone picker.  `zones` should be a comma-separated list
// of zone specs, where each zone spec is either in the format
// <zone>=<integer weight> or just <zone> (in which case the weight defaults to
// 1).
func newZonePicker(zones string) (*weightedRoundRobin, error) {
	if zones == "" {
		return nil, errors.New("ZONES must not be empty")
	}

	sw := &weighted.SW{}
	// Each zoneSpec should look like <string>=<integer>
	// or just <string> (with no "=").
	for _, zoneSpec := range strings.Split(zones, ",") {
		// Splits on the first occurrence of "=", if any.
		// The length of the returned slice is:
		//   1 if there is no "=" in zoneSpec, and
		//   2 if there is one or more "=" in zoneSpec.
		zoneAndWeight := strings.SplitN(zoneSpec, "=", 2)
		if len(zoneAndWeight) == 1 {
			// The weight defaults to 1 for any zone with no weight specfied.
			zoneAndWeight = append(zoneAndWeight, "1")
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
