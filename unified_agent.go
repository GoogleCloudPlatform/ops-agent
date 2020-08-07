// The unified_agent server reads a unified configuration and translates to
// specific configurations for a set of sub-agents.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
)

var (
	unifiedConfig = flag.String("unified_config", "", "path to read unified config")
	// TODO: Replace with flags for real sub-agents.
	subAgentConfig = flag.String("subagent_config", "", "path to write sub-agent config")
)

func sdNotify(state string) error {
	name := os.Getenv("NOTIFY_SOCKET")
	if name == "" {
		return fmt.Errorf("NOTIFY_SOCKET is empty")
	}

	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: name, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(state))
	return err
}

func translateConfig(cfg []byte) []byte {
	// TODO: Plug in real translation code.
	return cfg
}

type server struct {
	cfg []byte
}

func (s *server) rewriteSubAgentConfigs() error {
	log.Printf("Rewriting sub-agent configs")

	unifiedCfg, err := ioutil.ReadFile(*unifiedConfig)
	if err != nil {
		return err
	}

	// TODO: Support multiple sub-agents.
	subCfg := translateConfig(unifiedCfg)
	// TODO: Consider using target directory instead of "" in order to avoid
	// renaming file across devices.
	tmpfile, err := ioutil.TempFile("", "cfg")
	if err != nil {
		return err
	}
	if _, err := tmpfile.Write(subCfg); err != nil {
		return err
	}
	if err = os.Rename(tmpfile.Name(), *subAgentConfig); err != nil {
		return err
	}
	log.Printf("Rewrote sub-agent configs")

	s.cfg = unifiedCfg
	return nil
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Unified config is <%s>\n", bytes.TrimSpace(s.cfg))
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Unified agent starting")

	s := &server{}
	if err := s.rewriteSubAgentConfigs(); err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", s.handleRoot)

	if err := sdNotify("READY=1"); err != nil {
		log.Printf("Failed to notify ready: %s", err)
	}

	log.Printf("Unified agent entering main loop")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
