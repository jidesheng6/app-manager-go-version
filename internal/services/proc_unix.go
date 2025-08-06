//go:build !windows
// +build !windows

package services

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr 设置Unix/Linux系统进程属性
func (s *ApplicationService) setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // 创建新的进程组
	}
}