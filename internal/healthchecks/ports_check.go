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
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
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

func (c PortsCheck) RunCheck(logger *logs.FileLogger) error {
	fbErr := runFluentBitCheck(logger)
	otelErr := runOtelCollectorCheck(logger)
	return errors.Join(fbErr, otelErr)
}

func runFluentBitCheck(logger *logs.FileLogger) error {
	fbActive, err := isSubagentActive("google-cloud-ops-agent-fluent-bit")
	if err != nil {
		return err
	}
	if fbActive {
		return nil
	}

	// Fluent-bit listens on tcp4. Check for fluent-bit self metrics port.
	err = runPortCheck(logger, fluentbit.MetricsPort, tcpHost, "tcp4", FbMetricsPortErr)
	if err != nil {
		return err
	}
	return nil
}

func runOtelCollectorCheck(logger *logs.FileLogger) error {
	ocActive, err := isSubagentActive("google-cloud-ops-agent-opentelemetry-collector")
	if err != nil {
		return err
	}
	if ocActive {
		return nil
	}

	// Opentelemetry-collector listens in both tcp4 and tcp6. Check for opentelemetry-collector self metrics port.
	err = runPortCheck(logger, otel.MetricsPort, tcpHost, "tcp4", OtelMetricsPortErr)
	if err != nil {
		return err
	}

	err = runPortCheck(logger, otel.MetricsPort, tcp6Host, "tcp6", OtelMetricsPortErr)
	if err != nil {
		return err
	}
	return nil
}

func runPortCheck(logger *logs.FileLogger, port int, host, network string, healthCheckError error) error {
	available, err := checkIfPortAvailable(host, strconv.Itoa(port), network)
	if err != nil {
		return err
	}
	if !available {
		return healthCheckError
	}
	logger.Printf("listening to %s:", net.JoinHostPort(host, strconv.Itoa(port)))

	return nil
}
