// Copyright 2020 Google LLC
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

package main

import (
	"flag"
	"log"
	"os"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/shirou/gopsutil/host"
	"cloud.google.com/go/logging"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"

	"context"

    apikeys "cloud.google.com/go/apikeys/apiv2"
    "google.golang.org/api/iterator"

    apikeyspb "google.golang.org/genproto/googleapis/api/apikeys/v2"
)

var (
	service  = flag.String("service", "", "service to generate config for")
	outDir   = flag.String("out", os.Getenv("RUNTIME_DIRECTORY"), "directory to write configuration files to")
	input    = flag.String("in", "/etc/google-cloud-ops-agent/config.yaml", "path to the user specified agent config")
	logsDir  = flag.String("logs", "/var/log/google-cloud-ops-agent", "path to store agent logs")
	stateDir = flag.String("state", "/var/lib/google-cloud-ops-agent", "path to store agent state like buffers")

	// MetadataResource is the resource metadata for the instance we're running on.
	// Note: This is a global variable so that it can be set in tests.
	MetadataResource resourcedetector.Resource
)

func Health_Checks(project string) error {

	fmt.Println("Health_Checks")
	ctx := context.Background()
    // This snippet has been automatically generated and should be regarded as a code template only.
    // It will require modifications to work:
    // - It may require correct/in-range values for request initialization.
    // - It may require specifying regional endpoints when creating the service client as shown in:
    //   https://pkg.go.dev/cloud.google.com/go#hdr-Client_Options
    c, err := apikeys.NewClient(ctx)
    fmt.Println(c)
    if err != nil {
            // TODO: Handle error.
    }
    defer c.Close()

    req := &apikeyspb.ListKeysRequest{
    	Parent: "fcovalente-dev",
        // TODO: Fill request struct fields.
        // See https://pkg.go.dev/google.golang.org/genproto/googleapis/api/apikeys/v2#ListKeysRequest.
    }
    it := c.ListKeys(ctx, req)
    for {
            resp, err := it.Next()
            fmt.Println(resp)
            if err == iterator.Done {
                    break
            }
            if err != nil {
            		fmt.Println(err)
                    // TODO: Handle error.
                    break
            }
            // TODO: Use resp.
            _ = resp
    }

	return nil
}

func APIChecks(project string) error {

	ctx := context.Background()
    client, err := logging.NewClient(ctx, "my-project")
    if err != nil {
            // TODO: Handle error.
            fmt.Println(err)
    }
    if err := client.Ping(ctx); err != nil {
            // TODO: Handle error.
            fmt.Println(err)
    }

	return nil
}

func main() {
	flag.Parse()

	MetadataResource, err = resourcedetector.GetResource()
	if err != nil {
		return fmt.Errorf("can't get resource metadata: %w", err)
	}
	fmt.Println(MetadataResource.Project)

	if err := APIChecks(MetadataResource.Project); err != nil {
		log.Fatalf("APIChecks : %s", err)
	}
	
	if err := Health_Checks(MetadataResource.Project); err != nil {
		log.Fatalf("Health_Checks : %s", err)
	}

	/*if err := run(); err != nil {
		log.Fatalf("The agent config file is not valid. Detailed error: %s", err)
	}*/
}
func run() error {
	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	builtInConfig, mergedConfig, err := confgenerator.MergeConfFiles(*input, "linux", apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	// Log the built-in and merged config files to STDOUT. These are then written
	// by journald to var/log/syslog and so to Cloud Logging once the ops-agent is
	// running.
	log.Printf("Built-in config:\n%s", builtInConfig)
	log.Printf("Merged config:\n%s", mergedConfig)

	hostInfo, err := host.Info()
	if err != nil {
		return err
	}
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(mergedConfig, hostInfo.OS)
	if err != nil {
		return err
	}
	return confgenerator.GenerateFilesFromConfig(&uc, *service, *logsDir, *stateDir, *outDir)
}
