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

//go:build gpu
// +build gpu

package nvmlreceiver

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/nvmlreceiver/internal/metadata"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestScrapeOnLibraryNotFound(t *testing.T) {
	realNvmlInit := nvmlInit
	defer func() { nvmlInit = realNvmlInit }()
	nvmlInit = func() nvml.Return { panic("library not found") }

	scraper := newNvmlScraper(createDefaultConfig().(*Config), receivertest.NewNopSettings(metadata.Type))
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, metrics.MetricCount())
}
