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

//go:build gpu
// +build gpu

package dcgmreceiver

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/scraper"
	"go.opentelemetry.io/collector/scraper/scraperhelper"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/dcgmreceiver/internal/metadata"
)

var dcgmIDToName map[dcgm.Short]string
var randSource = rand.New(rand.NewSource(time.Now().UnixMicro()))

func init() {
	dcgmIDToName = make(map[dcgm.Short]string, len(dcgm.DCGM_FI))
	for fieldName, fieldID := range dcgm.DCGM_FI {
		if strings.HasPrefix(fieldName, "DCGM_FT_") {
			continue
		}
		dcgmIDToName[fieldID] = fieldName
	}
}

func createMetricsReceiver(
	_ context.Context,
	params receiver.Settings,
	rConf component.Config,
	consumer consumer.Metrics,
) (receiver.Metrics, error) {
	cfg, ok := rConf.(*Config)
	if !ok {
		return nil, fmt.Errorf("Unable to cast receiver configuration to dcgm.Config")
	}

	ns := newDcgmScraper(cfg, params)
	scp, err := scraper.NewMetrics(
		ns.scrape,
		scraper.WithStart(ns.start),
		scraper.WithShutdown(ns.stop))
	if err != nil {
		return nil, err
	}

	return scraperhelper.NewMetricsController(
		&cfg.ControllerConfig, params, consumer,
		scraperhelper.AddScraper(metadata.Type, scp),
	)
}
