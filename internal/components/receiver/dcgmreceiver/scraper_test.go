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

//go:build gpu && !has_gpu
// +build gpu,!has_gpu

package dcgmreceiver

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestScraperWithoutDcgm(t *testing.T) {
	var settings receiver.Settings
	var mu sync.Mutex
	seenDcgmNotInstalledWarning := false
	settings.Logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.Hooks(func(e zapcore.Entry) error {
		if e.Level == zap.WarnLevel && strings.Contains(e.Message, "Unable to connect to DCGM daemon at localhost:5555 on libdcgm.so not Found; Is the DCGM daemon running") {
			mu.Lock()
			seenDcgmNotInstalledWarning = true
			mu.Unlock()
		}
		return nil
	})))

	scraper := newDcgmScraper(createDefaultConfig().(*Config), settings)
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())
	mu.Lock()
	assert.Equal(t, true, seenDcgmNotInstalledWarning)
	mu.Unlock()
	assert.NoError(t, err) // If failed to init DCGM, should have no error
	assert.Equal(t, 0, metrics.MetricCount())

	// Scrape again with DCGM not available
	metrics, err = scraper.scrape(context.Background())
	mu.Lock()
	assert.Equal(t, true, seenDcgmNotInstalledWarning)
	mu.Unlock()
	assert.NoError(t, err)
	assert.Equal(t, 0, metrics.MetricCount())

	err = scraper.stop(context.Background())
	assert.NoError(t, err)
}
