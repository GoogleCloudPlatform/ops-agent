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

    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)



type PortsCheck struct{
    HealthCheck
}

func (c PortsCheck) check_port(host string, port string) error {
    lsnr, err := net.Listen("tcp", net.JoinHostPort(host, port))
    if err != nil {
        c.LogMessage(fmt.Sprintf("==> Connection Error: %s ", err))
        return err
    }
    c.LogMessage(fmt.Sprintf("==> Listening on: %s", lsnr.Addr()))
    if lsnr != nil {
        defer lsnr.Close()
        c.LogMessage(fmt.Sprintf("==> Opened %s",net.JoinHostPort(host, port)))    
    } else {
        return fmt.Errorf("Lister not opened.")
    }
    return nil
}

func (c PortsCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {
    // Check prometheus exporter host port : 0.0.0.0 : 20202
    host := "0.0.0.0"
    port := "20202"
    err := c.check_port(host, port)
    if err != nil {
        c.Fail("Listening to : " + net.JoinHostPort(host, port) + " was not successful.",
            fmt.Sprintf("Check the port : %s is free", port))
        // return err
    }
    return nil
}

func init() {
    GCEHealthChecks.RegisterCheck("Ports Check", &PortsCheck{HealthCheck: NewHealthCheck()})
}