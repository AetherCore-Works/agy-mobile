//go:build !windows

package cockpit

import "os/exec"

func configureOSProcess(_ *exec.Cmd) {}
