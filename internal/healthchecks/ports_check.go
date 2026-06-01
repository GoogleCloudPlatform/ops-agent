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
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/portutil"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
)

const (
	tcpHost  = "0.0.0.0"
	tcp6Host = "::"
)

type PortsCheck struct{}

func (c PortsCheck) Name() string {
	return "Ports Check"
}

// checkIfPortAvailable listens in the provided socket and local provided network (tcp4, tcp6, ...)
// and handles the errors if the port is already being used by another process.
func checkIfPortAvailable(host string, port string, network string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var lc net.ListenConfig
	lsnr, err := lc.Listen(ctx, network, net.JoinHostPort(host, port))
	if err != nil {
		if isPortUnavailableError(err) {
			return false, nil
		}
		return false, fmt.Errorf("error listening to: %s, detail: %w", net.JoinHostPort(host, port), err)
	}
	lsnr.Close()
	return true, nil
}

func (c PortsCheck) RunCheck(logger logs.StructuredLogger) error {
	return runOtelCollectorCheck(logger)
}

func runOtelCollectorCheck(logger logs.StructuredLogger) error {
	ocActive, err := isSubagentActive("google-cloud-ops-agent")
	if err != nil {
		return err
	}
	if ocActive {
		return nil
	}

	// Opentelemetry-collector listens in both tcp4 and tcp6. Check for opentelemetry-collector self metrics port.
	err = runPortCheck(logger, int(portutil.GetPortFromEnv(otel.ExperimentalMetricsPortEnv, otel.MetricsPort)), tcpHost, "tcp4", OtelMetricsPortErr)
	if err != nil {
		return err
	}

	err = runPortCheck(logger, int(portutil.GetPortFromEnv(otel.ExperimentalMetricsPortEnv, otel.MetricsPort)), tcp6Host, "tcp6", OtelMetricsPortErr)
	if err != nil {
		return err
	}
	return nil
}

func runPortCheck(logger logs.StructuredLogger, port int, host, network string, healthCheckError error) error {
	available, err := checkIfPortAvailable(host, strconv.Itoa(port), network)
	if err != nil {
		return err
	}
	if !available {
		if hcErr, ok := healthCheckError.(HealthCheckError); ok {
			hcErr.Message = fmt.Sprintf("Port %d needed for Ops Agent self metrics is unavailable.", port)
			hcErr.Action = fmt.Sprintf("Verify that port %d is open.", port)
			return hcErr
		}
		return healthCheckError
	}
	logger.Infof("listening to %s:", net.JoinHostPort(host, strconv.Itoa(port)))

	return nil
}
