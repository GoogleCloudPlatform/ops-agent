// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret_test

import (
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
	"github.com/goccy/go-yaml"
)

func TestSecretStringStringer(t *testing.T) {
	var s secret.String = "My credit card number!"
	result := s.String()
	if !strings.Contains(result, secret.RedactedValue) {
		t.Fatalf("expected result to be redacted, instead was \"%s\"", result)
	}
}

func TestSecretStringMarshalYAML(t *testing.T) {
	type x struct {
		S secret.String `yaml:"s"`
	}

	testX := x{S: "My credit card number!"}
	resultBytes, err := yaml.Marshal(testX)
	result := string(resultBytes)
	if err != nil {
		t.Fatalf("expected marshal not to error, got: %s", result)
	}
	if !strings.Contains(result, secret.RedactedValue) {
		t.Fatalf("expected Marshal to redact secret field, got: %s", result)
	}
}

func TestSecretStringUnmarshalYAML(t *testing.T) {
	type x struct {
		S secret.String `yaml:"s"`
	}

	yml := "s: My credit card number!"
	var result x
	err := yaml.Unmarshal([]byte(yml), &result)
	if err != nil {
		t.Fatalf("expected marshal not to error, got: %s", result)
	}
	if result.S != "My credit card number!" {
		t.Fatalf("expected Unmarshal to retain secret field value, got: %s", result.S)
	}
}
