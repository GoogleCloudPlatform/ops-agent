// Copyright 2022 Google LLC
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

//go:build integration_test

package util

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
)

func GetConfigPath(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return `C:\Program Files\Google\Cloud Operations\Ops Agent\config\config.yaml`
	}
	return "/etc/google-cloud-ops-agent/config.yaml"
}

func GetOtelConfigPath(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return `C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\otel\otel.yaml`
	}
	return "/var/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml"
}

// DumpPointerArray formats the given array of pointers-to-structs as a strings
// using the given format, rather than just formatting them as addresses.
// format is usually either "%v" or "%+v".
func DumpPointerArray[T any](array []*T, format string) string {
	s := "["
	for i, element := range array {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf(format, element)
	}
	s += "]"
	return s
}
