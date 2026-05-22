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

	// Test 1: Env empty
	os.Unsetenv(envVar)
	if port := portutil.GetPortFromEnv(envVar, defaultPort); port != defaultPort {
		t.Errorf("Expected port %d when env is empty, got %d", defaultPort, port)
	}

	// Test 2: Valid port
	os.Setenv(envVar, "54321")
	if port := portutil.GetPortFromEnv(envVar, defaultPort); port != 54321 {
		t.Errorf("Expected port 54321, got %d", port)
	}

	// Test 3: Invalid port (not a number)
	os.Setenv(envVar, "invalid")
	if port := portutil.GetPortFromEnv(envVar, defaultPort); port != defaultPort {
		t.Errorf("Expected port %d for invalid env value, got %d", defaultPort, port)
	}

	// Test 4: Out of range port
	os.Setenv(envVar, "65536")
	if port := portutil.GetPortFromEnv(envVar, defaultPort); port != defaultPort {
		t.Errorf("Expected port %d for out-of-range env value, got %d", defaultPort, port)
	}

	// Clean up
	os.Unsetenv(envVar)
}
