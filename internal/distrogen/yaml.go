// Copyright 2025 Google LLC
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

package main

import (
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

func yamlUnmarshalFromFile[T any](path string) (*T, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result T
	err = yaml.Unmarshal(content, &result)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", path, err)
	}
	return &result, nil
}

func yamlUnmarshalFromFileInto[T any](path string, value *T) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(content, &value)
	if err != nil {
		return fmt.Errorf("error parsing %s: %w", path, err)
	}
	return nil
}

func yamlMarshalToFile[T any](value *T, path string, mode fs.FileMode) error {
	content, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

func renderYaml(value any) string {
	content, _ := yaml.Marshal(value)
	return string(content)
}

func mapMerge[K comparable, V any](m map[K]V, m2 map[K]V) {
	if m == nil {
		m = make(map[K]V, len(m2))
	}
	for k, v := range m2 {
		m[k] = v
	}
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
