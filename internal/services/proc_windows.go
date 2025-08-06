//go:build windows
// +build windows

package services

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr 设置Windows系统进程属性
func (s *ApplicationService) setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}