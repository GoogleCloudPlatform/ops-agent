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

//go:build windows
// +build windows

package windows

import (
	"log"

	"golang.org/x/sys/windows/registry"
)

func Is2012() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		log.Fatalf("could not open CurrentVersion key: %v", err)
	}
	defer key.Close()
	data, _, err := key.GetStringValue("CurrentBuildNumber")
	if err != nil {
		log.Fatalf("could not read CurrentBuildNumber: %v", err)
	}
	// https://en.wikipedia.org/wiki/List_of_Microsoft_Windows_versions#Server_versions
	return data == "9200" || data == "9600"
}
