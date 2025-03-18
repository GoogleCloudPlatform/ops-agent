package main

import (
	"encoding/base64"
	"fmt"
	"log"

	"golang.org/x/text/encoding/unicode"
)

func wrapPowershellCommand(command string) (string, error) {
	uni := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	encoded, err := uni.NewEncoder().String(command)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("powershell -NonInteractive -EncodedCommand %q", base64.StdEncoding.EncodeToString([]byte(encoded))), nil
}

func main() {

	installCmd := `gsutil cp gs://ops-agents-public-buckets-vendored-deps/mirrored-content/grpcurl/v1.8.6/grpcurl_1.8.6_windows_x86_64.zip C:\agentPlugin;Expand-Archive -Path "C:\agentPlugin\grpcurl_1.8.6_windows_x86_64.zip" -DestinationPath "C:\" -Force;$env:Path += ";C:\"`
	encodedCmd, err := wrapPowershellCommand(installCmd)
	if err != nil {
		log.Fatalf("Failed to wrap command: %v", err)
	}
	log.Printf("Encoded command: %s", encodedCmd)

}
