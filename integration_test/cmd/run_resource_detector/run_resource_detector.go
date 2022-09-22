package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
)

func main() {
	resource, err := resourcedetector.GetResource()
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	b, err := json.Marshal(resource)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
