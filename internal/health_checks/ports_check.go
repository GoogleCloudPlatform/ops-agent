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
	"fmt"
	"net"
)

type PortsCheck struct {
	HealthCheck
}

func (c PortsCheck) check_port(host string, port string) error {
	lsnr, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		compositeError := fmt.Errorf("connection Error : %w", err)
		c.Error(compositeError)
		return compositeError
	}
	if lsnr != nil {
		defer lsnr.Close()
		c.Log(fmt.Sprintf("opened %s", net.JoinHostPort(host, port)))
	} else {
		c.Fail("port-unavailable")
	}
	return nil
}

func (c PortsCheck) RunCheck() error {
	// TODO : Get ports from UnifiedConfig
	// Check prometheus exporter host port : 0.0.0.0 : 20202
	host := "0.0.0.0"
	port := "20202"
	err := c.check_port(host, port)
	if err != nil {
		c.Error(err)
		return err
	}
	return nil
}

func init() {
	GCEHealthChecks.RegisterCheck("Ports Check", &PortsCheck{HealthCheck: NewHealthCheck()})
}
