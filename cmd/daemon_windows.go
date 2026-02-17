//go:build windows

package cmd

import "syscall"

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Windows doesn't support Setsid, use CREATE_NEW_PROCESS_GROUP instead
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}
