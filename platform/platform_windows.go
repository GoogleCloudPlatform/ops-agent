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

package platform

import (
	"log"

	"golang.org/x/sys/windows/registry"
)

func getWindowsBuildNumber() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		log.Fatalf("could not open CurrentVersion key: %v", err)
	}
	defer key.Close()
	build, _, err := key.GetStringValue("CurrentBuildNumber")
	if err != nil {
		log.Fatalf("could not read CurrentBuildNumber: %v", err)
	}
	return build
}

func (p *Platform) detectPlatform() {
	p.Type = Windows
	p.WindowsBuildNumber = getWindowsBuildNumber()
	var err error
	p.WinlogV1Channels, err = getOldWinlogChannels()
	if err != nil {
		// Ignore the error, to preserve existing behavior.
		log.Printf("could not find Windows Event Log V1 channels: %v", err)
	}
}

// getOldWinlogChannels returns the set of event logs (channels) under
// HKLM\SYSTEM\CurrentControlSet\Services\EventLog, which (supposedly) corresponds
// to the available channels on the machine which are compatible with the "old API".
func getOldWinlogChannels() ([]string, error) {
	parentKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\EventLog`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer parentKey.Close()
	subKeys, err := parentKey.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	return subKeys, nil
}
