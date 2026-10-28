// A native Windows test fixture: lifecycle calls delegate to the real guard,
// while the tool check deliberately crashes, emits bad JSON, or hangs.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
)

func main() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}
	var event struct {
		Name string `json:"hook_event_name"`
	}
	if json.Unmarshal(input, &event) != nil {
		os.Exit(1)
	}
	if event.Name == "PreToolUse" {
		switch os.Getenv("SPS_DSH_FAULT_MODE") {
		case "crash":
			os.Exit(17)
		case "malformed":
			os.Stdout.WriteString("[]")
			return
		case "timeout":
			time.Sleep(time.Hour)
			return
		default:
			os.Exit(1)
		}
	}
	cmd := exec.Command(os.Getenv("SPS_DSH_REAL_BINARY"))
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
