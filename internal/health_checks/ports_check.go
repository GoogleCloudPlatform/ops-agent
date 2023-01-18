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
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"syscall"

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

func isErrorAddressAlreadyInUse(err error) bool {
	var eOsSyscall *os.SyscallError
	if !errors.As(err, &eOsSyscall) {
		return false
	}
	var errErrno syscall.Errno
	if !errors.As(eOsSyscall, &errErrno) {
		return false
	}
	if errErrno == syscall.EADDRINUSE {
		return true
	}
	const WSAEADDRINUSE = 10048
	if runtime.GOOS == "windows" && errErrno == WSAEADDRINUSE {
		return true
	}
	return false
}

func checkPortAvailable(host string, port string) (bool, error) {
	lsnr, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil && isErrorAddressAlreadyInUse(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("error listening to: %s, detail: %w", net.JoinHostPort(host, port), err)
	}
	lsnr.Close()
	return true, nil
}

func (c PortsCheck) RunCheck(logger *log.Logger) error {
	// Check self metrics ports
	self_metrics_host := "0.0.0.0"

	// Check for fluent-bit self metrics port
	available, err := checkPortAvailable(self_metrics_host, strconv.Itoa(fluentbit.MetricsPort))
	if err != nil {
		return err
	}
	if err == nil && !available {
		return FB_METRICS_PORT_ERR
	}
	logger.Printf("listening to %s:", net.JoinHostPort(self_metrics_host, strconv.Itoa(fluentbit.MetricsPort)))

	// Check for opentelemetry-collector self metrics port
	available, err = checkPortAvailable(self_metrics_host, strconv.Itoa(otel.MetricsPort))
	if err != nil {
		return err
	}
	if err == nil && !available {
		return OTEL_METRICS_PORT_ERR
	}
	logger.Printf("listening to %s:", net.JoinHostPort(self_metrics_host, strconv.Itoa(otel.MetricsPort)))

	return nil
}
