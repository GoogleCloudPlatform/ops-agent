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

func (c PortsCheck) RunCheck(logger *log.Logger) error {
	fbActive, err := isSubagentActive("google-cloud-ops-agent-fluent-bit")
	if err != nil {
		return err
	}
	ocActive, err := isSubagentActive("google-cloud-ops-agent-opentelemetry-collector")
	if err != nil {
		return err
	}
	if fbActive && ocActive {
		return nil
	}

	// fluent-bit listens on tcp4. opentelemetry-collector listens in both tcp4 and tcp6.
	tcpHost := "0.0.0.0"
	tcp6Host := "::"

	// Check for fluent-bit self metrics port
	available, err := checkIfPortAvailable(tcpHost, strconv.Itoa(fluentbit.MetricsPort), "tcp4")
	if err != nil {
		return err
	}
	if !available {
		return FbMetricsPortErr
	}
	logger.Printf("listening to %s:", net.JoinHostPort(tcpHost, strconv.Itoa(fluentbit.MetricsPort)))

	// Check for opentelemetry-collector self metrics port
	available, err = checkIfPortAvailable(tcpHost, strconv.Itoa(otel.MetricsPort), "tcp4")
	if err != nil {
		return err
	}
	if !available {
		return OtelMetricsPortErr
	}
	logger.Printf("listening to %s:", net.JoinHostPort(tcpHost, strconv.Itoa(otel.MetricsPort)))

	available, err = checkIfPortAvailable(tcp6Host, strconv.Itoa(otel.MetricsPort), "tcp6")
	if err != nil {
		return err
	}
	if !available {
		return OtelMetricsPortErr
	}
	logger.Printf("listening to %s:", net.JoinHostPort(tcp6Host, strconv.Itoa(otel.MetricsPort)))

	return nil
}
