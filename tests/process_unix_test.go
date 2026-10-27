//go:build !windows

package host_test

import "os/exec"

func processGroup() (func(*exec.Cmd) error, func(), error) {
	return func(*exec.Cmd) error { return nil }, func() {}, nil
}
