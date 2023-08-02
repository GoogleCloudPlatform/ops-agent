// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows

package accelerators

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	nvidiaVendorId   = "0x10de"
	sysDevicesPath   = "/sys/bus/pci/devices"
	deviceVendorPath = "vendor"
)

// HasNvidiaGpu scans the /sys PCI device tree and compare the vendor ID with
// the vendor ID of NVIDIA. to determine if the current system has any GPU, and
// return errors if failed to scan the vendor files
func HasNvidiaGpu() (bool, error) {
	if devices, err := os.ReadDir(sysDevicesPath); err != nil {
		return false, err
	} else {
		for _, device := range devices {
			vendorFile := filepath.Join(sysDevicesPath, device.Name(), deviceVendorPath)
			vendor, err := os.ReadFile(vendorFile)
			if err != nil {
				return false, err
			}
			if strings.EqualFold(strings.TrimSpace(string(vendor)), nvidiaVendorId) {
				return true, nil
			}
		}
	}
	return false, nil
}
