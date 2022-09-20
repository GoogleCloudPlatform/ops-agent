// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows && !linux
// +build !windows,!linux

package pagingscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/pagingscraper"

import "github.com/shirou/gopsutil/v3/mem"

func getPageFileStats() ([]*pageFileStats, error) {
	vmem, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	return []*pageFileStats{{
		deviceName:  "", // We do not support per-device swap
		usedBytes:   vmem.SwapTotal - vmem.SwapFree - vmem.SwapCached,
		freeBytes:   vmem.SwapFree,
		cachedBytes: &vmem.SwapCached,
	}}, nil
}
