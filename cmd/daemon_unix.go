//go:build !windows

package cmd

import "syscall"

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}
}
