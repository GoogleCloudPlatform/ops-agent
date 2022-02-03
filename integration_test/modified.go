package integration_test

import (
	"os/exec"
	"strings"
	"testing"
)

func modifiedFiles(t *testing.T) []string {
	cmd := exec.Command("git", "diff", "--name-only", "origin/master")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("got error calling `git diff`: %v", err)
	}

	return strings.Split(string(out), "\n")
}
