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
	"errors"
	"fmt"
)

var ErrSectionNotFound = errors.New("could not find section")

// ComponentsFromOTelConfig will take an OpenTelemetry Collector
// configuration map and load the component names that would
// be loaded from a registry on distribution generation.
func ComponentsFromOTelConfig(otelConfig map[string]any) (*DistributionComponents, error) {
	components := &DistributionComponents{}
	var err error
	components.Receivers, err = readComponentsFromSection("receivers", otelConfig)
	if err != nil && !errors.Is(err, ErrSectionNotFound) {
		return nil, err
	}
	components.Processors, err = readComponentsFromSection("processors", otelConfig)
	if err != nil && !errors.Is(err, ErrSectionNotFound) {
		return nil, err
	}
	components.Exporters, err = readComponentsFromSection("exporters", otelConfig)
	if err != nil && !errors.Is(err, ErrSectionNotFound) {
		return nil, err
	}
	components.Connectors, err = readComponentsFromSection("connectors", otelConfig)
	if err != nil && !errors.Is(err, ErrSectionNotFound) {
		return nil, err
	}
	components.Extensions, err = readComponentsFromSection("extensions", otelConfig)
	if err != nil && !errors.Is(err, ErrSectionNotFound) {
		return nil, err
	}
	return components, nil
}

func readComponentsFromSection(sectionName string, otelConfig map[string]any) ([]string, error) {
	var section map[string]any
	rawSection, ok := otelConfig[sectionName]
	if !ok {
		return nil, fmt.Errorf("reading section %s: %w", sectionName, ErrSectionNotFound)
	}
	section, ok = rawSection.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("reading section %s: invalid section data", sectionName)
	}
	return mapKeys(section), nil
}
