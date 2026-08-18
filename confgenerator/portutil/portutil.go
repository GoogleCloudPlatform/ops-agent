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

package portutil

import (
	"os"
	"strconv"
)

// GetPortFromEnv retrieves a port number from the specified environment variable.
// If the environment variable is not set or contains an invalid port number,
// it returns the defaultPort.
func GetPortFromEnv(envVar string, defaultPort uint16) uint16 {
	if portStr := os.Getenv(envVar); portStr != "" {
		if port, err := strconv.ParseUint(portStr, 10, 16); err == nil {
			return uint16(port)
		}
	}
	return defaultPort
}
