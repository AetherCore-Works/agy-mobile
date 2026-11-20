//go:build !windows

package main

func maybeRunConsoleController() bool {
	return false
}
