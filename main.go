package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

const maxJSON = 16 << 20

func decode(r io.Reader, v any) error {
	data, err := io.ReadAll(io.LimitReader(r, maxJSON+1))
	if err != nil {
		return err
	}
	if len(data) > maxJSON {
		return fmt.Errorf("input too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

func run(in io.Reader, out, stderr io.Writer) (code int) {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(stderr, "Issue / PR guard: runtime failure; denied")
			code = 2
		}
	}()
	var event *Event
	err := decode(in, &event)
	if err == nil && (event == nil || event.Name == "") {
		err = fmt.Errorf("expected event object with hook_event_name")
	}
	if err == nil {
		var result map[string]any
		result, err = handle(*event)
		if err == nil {
			err = json.NewEncoder(out).Encode(result)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "Issue / PR guard 检查失败，未放行：", err)
		return 2
	}
	return 0
}

func main() {
	// Exit before the host timeout, which may otherwise continue the tool call.
	deadline := time.AfterFunc(2500*time.Millisecond, func() {
		fmt.Fprintln(os.Stderr, "Issue / PR guard: deadline exceeded; denied")
		os.Exit(2)
	})
	code := run(os.Stdin, os.Stdout, os.Stderr)
	deadline.Stop()
	os.Exit(code)
}
