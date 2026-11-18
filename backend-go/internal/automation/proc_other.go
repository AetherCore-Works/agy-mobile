//go:build !windows

package automation

import "os/exec"

func configureOSProcess(_ *exec.Cmd) {}
