package main

import (
	"log"

	"golang.org/x/sys/windows/svc"
)

const dataDirectory = `Google/Cloud Operations/Ops Agent`
const serviceName = "Google Cloud Ops Agent"

func main() {
	if ok, err := svc.IsWindowsService(); ok && err == nil {
		if err := run(serviceName); err != nil {
			log.Fatal(err)
		}
	} else if err != nil {
		log.Fatalf("failed to talk to service control manager: %v", err)
	} else {
		if err := install(); err != nil {
			log.Fatal(err)
		}
	}
}
