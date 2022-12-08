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

package health_checks

import (
	"log"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type PortsCheck struct {
	Config confgenerator.UnifiedConfig
}

func (c PortsCheck) Name() string {
	return "Ports Check"
}

func (c PortsCheck) check_port(host string, port string) error {
	lsnr, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil && strings.HasSuffix(err.Error(), "bind: address already in use") {
		return PORT_UNAVAILABLE_ERR
	}
	if err != nil {
		return fmt.Errorf("connection Error : %w", err)
	}
	if lsnr != nil {
		defer lsnr.Close()
		log.Printf("opened %s", net.JoinHostPort(host, port))
	} else {
		return PORT_UNAVAILABLE_ERR
	}
	return nil
}

func (c PortsCheck) RunCheck() error {
	// TODO : Get ports from UnifiedConfig
	self_metrics_host := "0.0.0.0"

	// Check for fluent-bit self metrics port
	err := c.check_port(self_metrics_host, strconv.Itoa(fluentbit.MetricsPort))
	if err != nil {
		return err
	}

	// Check for opentelemetry-collector self metrics port
	err = c.check_port(self_metrics_host, strconv.Itoa(otel.MetricsPort))
	if err != nil {
		return err
	}
	return nil
}