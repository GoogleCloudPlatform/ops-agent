// Copyright 2026 Google LLC
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

package portutil_test

import (
	"os"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/portutil"
)

func TestGetPortFromEnv(t *testing.T) {
	envVar := "TEST_PORT_ENV"
	defaultPort := uint16(12345)

	tests := []struct {
		name         string
		setupEnv     func()
		expectedPort uint16
	}{
		{
			name: "Env empty",
			setupEnv: func() {
				os.Unsetenv(envVar)
			},
			expectedPort: defaultPort,
		},
		{
			name: "Valid port",
			setupEnv: func() {
				os.Setenv(envVar, "54321")
			},
			expectedPort: 54321,
		},
		{
			name: "Invalid port (not a number)",
			setupEnv: func() {
				os.Setenv(envVar, "invalid")
			},
			expectedPort: defaultPort,
		},
		{
			name: "Out of range port",
			setupEnv: func() {
				os.Setenv(envVar, "65536")
			},
			expectedPort: defaultPort,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupEnv()
			defer os.Unsetenv(envVar)

			got := portutil.GetPortFromEnv(envVar, defaultPort)
			if got != tc.expectedPort {
				t.Errorf("GetPortFromEnv() = %d, want %d", got, tc.expectedPort)
			}
		})
	}
}
