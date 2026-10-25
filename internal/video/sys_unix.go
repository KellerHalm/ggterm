//go:build !windows

package video

import "syscall"

func sysProcAttr() *syscall.SysProcAttr {
	return nil
}
