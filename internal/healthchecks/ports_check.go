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

package healthchecks

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type PortsCheck struct{}

func (c PortsCheck) Name() string {
	return "Ports Check"
}

// checkPortAvailable listens in the provided socket and local provided network (tcp4, tcp6, ...)
// and handles the errors if the port is already being used by another process.
func checkPortAvailable(host string, port string, network string) (bool, error) {
	lsnr, err := net.Listen(network, net.JoinHostPort(host, port))
	if err != nil {
		if isPortUnavailableError(err) {
			return false, nil
		}
		return false, fmt.Errorf("error listening to: %s, detail: %w", net.JoinHostPort(host, port), err)
	}
	lsnr.Close()
	return true, nil
}

func (c PortsCheck) RunCheck(logger *log.Logger) error {
	// fluent-bit listens on tcp4. opentelemetry-collector listens in both tcp4 and tcp6.
	self_metrics_host_tcp4 := "0.0.0.0"
	self_metrics_host_tcp6 := "::"

	// Check for fluent-bit self metrics port
	available, err := checkPortAvailable(self_metrics_host_tcp4, strconv.Itoa(fluentbit.MetricsPort), "tcp4")
	if err != nil {
		return err
	}
	if !available {
		return FB_METRICS_PORT_ERR
	}
	logger.Printf("listening to %s:", net.JoinHostPort(self_metrics_host_tcp4, strconv.Itoa(fluentbit.MetricsPort)))

	// Check for opentelemetry-collector self metrics port
	available, err = checkPortAvailable(self_metrics_host_tcp4, strconv.Itoa(otel.MetricsPort), "tcp4")
	if err != nil {
		return err
	}
	if !available {
		return OTEL_METRICS_PORT_ERR
	}
	logger.Printf("listening to %s:", net.JoinHostPort(self_metrics_host_tcp4, strconv.Itoa(otel.MetricsPort)))

	available, err = checkPortAvailable(self_metrics_host_tcp6, strconv.Itoa(otel.MetricsPort), "tcp6")
	if err != nil {
		return err
	}
	if !available {
		return OTEL_METRICS_PORT_ERR
	}
	logger.Printf("listening to %s:", net.JoinHostPort(self_metrics_host_tcp6, strconv.Itoa(otel.MetricsPort)))

	return nil
}
