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

func check_port(host string, port string) error {
    lsnr, err := net.Listen("tcp", net.JoinHostPort(host, port))
    if err != nil {
        fmt.Println("==> Connection Error:", err)
        return err
    }
    fmt.Println("==> Listening on:", lsnr.Addr())
    if lsnr != nil {
        defer lsnr.Close()
        fmt.Println("==> Opened", net.JoinHostPort(host, port))    
    }
    return nil
}

type PortsCheck struct{}

func (c PortsCheck) RunCheck(uc *confgenerator.UnifiedConfig) (string, error) {
    fmt.Println("\n> PortsCheck \n \n")

    // Check prometheus exporter host port : 0.0.0.0 : 20202
    host := "0.0.0.0"
    port := "20202"
    err := check_port(host, port)
    if err != nil {
        return "", err
    }
    return "PASS", nil
}

func init() {
    GCEHealthChecks.RegisterCheck("ports_check", &PortsCheck{})
}