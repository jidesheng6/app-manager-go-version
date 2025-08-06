package models

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Application 应用程序模型
type Application struct {
	Name     string    // 应用名称
	FilePath string    // JAR/WAR文件路径
	PID      int       // 进程ID
	Status   AppStatus // 运行状态
	StartTime time.Time // 启动时间
	LogFile  string    // 日志文件路径
	PIDFile  string    // PID文件路径
	ConfigFile string  // 配置文件路径
}

// AppStatus 应用状态枚举
type AppStatus int

const (
	// StatusStopped 已停止
	StatusStopped AppStatus = iota
	// StatusRunning 运行中
	StatusRunning
	// StatusStarting 启动中
	StatusStarting
	// StatusStopping 停止中
	StatusStopping
	// StatusUnknown 未知状态
	StatusUnknown
)

// String 返回状态的字符串表示
func (s AppStatus) String() string {
	switch s {
	case StatusStopped:
		return "已停止"
	case StatusRunning:
		return "运行中"
	case StatusStarting:
		return "启动中"
	case StatusStopping:
		return "停止中"
	default:
		return "未知"
	}
}

// NewApplication 创建新的应用实例
func NewApplication(filePath string) *Application {
	name := getAppNameFromPath(filePath)
	
	return &Application{
		Name:     name,
		FilePath: filePath,
		Status:   StatusStopped,
	}
}

// getAppNameFromPath 从文件路径提取应用名称
func getAppNameFromPath(filePath string) string {
	fileName := filepath.Base(filePath)
	// 移除文件扩展名
	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	// 移除版本号后缀（如 -1.0.0-SNAPSHOT）
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		if isVersionSuffix(name[idx+1:]) {
			name = name[:idx]
		}
	}
	return name
}

// isVersionSuffix 判断是否为版本号后缀
func isVersionSuffix(suffix string) bool {
	// 检查是否包含数字和点号的组合，或者是SNAPSHOT
	if strings.Contains(suffix, "SNAPSHOT") {
		return true
	}
	
	// 简单的版本号检查（如 1.0.0）
	parts := strings.Split(suffix, ".")
	if len(parts) >= 2 {
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				return false
			}
		}
		return true
	}
	
	return false
}

// IsRunning 检查应用是否正在运行
func (app *Application) IsRunning() bool {
	if app.PID <= 0 {
		return false
	}
	
	if runtime.GOOS == "windows" {
		// Windows系统使用tasklist命令检查进程是否存在
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", app.PID), "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		
		// 如果输出包含进程信息，说明进程存在
		outputStr := strings.TrimSpace(string(output))
		return outputStr != "" && !strings.Contains(outputStr, "INFO: No tasks are running")
	} else {
		// Unix系统使用多种方法检查进程是否存在
		
		// 方法1: 检查/proc/PID目录是否存在
		procDir := fmt.Sprintf("/proc/%d", app.PID)
		if _, err := os.Stat(procDir); err == nil {
			// 进一步验证这是一个Java进程
			cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", app.PID)
			if cmdlineData, err := os.ReadFile(cmdlineFile); err == nil {
				cmdline := string(cmdlineData)
				if strings.Contains(cmdline, "java") && strings.Contains(cmdline, ".jar") {
					return true
				}
			}
		}
		
		// 方法2: 使用ps命令验证
		cmd := exec.Command("ps", "-p", strconv.Itoa(app.PID), "-o", "pid", "--no-headers")
		output, err := cmd.Output()
		if err == nil {
			outputStr := strings.TrimSpace(string(output))
			if outputStr != "" {
				return true
			}
		}
		
		// 方法3: 使用信号检查（作为最后的备选）
		process, err := os.FindProcess(app.PID)
		if err != nil {
			return false
		}
		
		// 发送信号0来检查进程是否存在
		err = process.Signal(syscall.Signal(0))
		return err == nil
	}
}

// LoadPID 从PID文件加载进程ID
func (app *Application) LoadPID(pidFile string) error {
	app.PIDFile = pidFile
	
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		app.PID = 0
		return nil
	}
	
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("读取PID文件失败: %w", err)
	}
	
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("解析PID失败: %w", err)
	}
	
	app.PID = pid
	return nil
}

// SavePID 保存进程ID到文件
func (app *Application) SavePID(pidFile string) error {
	app.PIDFile = pidFile
	
	// 确保目录存在
	dir := filepath.Dir(pidFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建PID目录失败: %w", err)
	}
	
	// 写入PID
	pidStr := strconv.Itoa(app.PID)
	if err := os.WriteFile(pidFile, []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("写入PID文件失败: %w", err)
	}
	
	return nil
}

// RemovePID 删除PID文件
func (app *Application) RemovePID() error {
	if app.PIDFile == "" {
		return nil
	}
	
	if err := os.Remove(app.PIDFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除PID文件失败: %w", err)
	}
	
	app.PID = 0
	return nil
}

// UpdateStatus 更新应用状态
func (app *Application) UpdateStatus() {
	if app.IsRunning() {
		app.Status = StatusRunning
	} else {
		app.Status = StatusStopped
		app.PID = 0
	}
}

// GetUptime 获取运行时长
func (app *Application) GetUptime() time.Duration {
	if !app.IsRunning() || app.StartTime.IsZero() {
		return 0
	}
	return time.Since(app.StartTime)
}

// GetUptimeString 获取运行时长的字符串表示
func (app *Application) GetUptimeString() string {
	uptime := app.GetUptime()
	if uptime == 0 {
		return "--"
	}
	
	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60
	
	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

// ProcessInfo 进程信息结构
type ProcessInfo struct {
	CPUPercent    float64
	MemoryKB      int64
	MemoryPercent float64
}

// GetProcessInfo 获取进程资源使用信息
func (app *Application) GetProcessInfo() *ProcessInfo {
	if !app.IsRunning() {
		return &ProcessInfo{}
	}
	
	if runtime.GOOS == "windows" {
		return app.getProcessInfoWindows()
	} else {
		return app.getProcessInfoLinux()
	}
}

// getProcessInfoLinux 在Linux系统上获取进程信息
func (app *Application) getProcessInfoLinux() *ProcessInfo {
	info := &ProcessInfo{}
	
	// 使用ps命令获取进程信息，增加更多字段
	cmd := exec.Command("ps", "-p", strconv.Itoa(app.PID), "-o", "pid,pcpu,pmem,rss,vsz", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		// 如果ps命令失败，尝试从/proc读取
		return app.getProcessInfoFromProc()
	}
	
	line := strings.TrimSpace(string(output))
	if line == "" {
		return info
	}
	
	fields := strings.Fields(line)
	if len(fields) >= 4 {
		// CPU使用率
		if cpu, err := strconv.ParseFloat(fields[1], 64); err == nil {
			info.CPUPercent = cpu
		}
		
		// 内存使用率
		if mem, err := strconv.ParseFloat(fields[2], 64); err == nil {
			info.MemoryPercent = mem
		}
		
		// 内存大小(KB)
		if rss, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
			info.MemoryKB = rss
		}
	}
	
	return info
}

// getProcessInfoFromProc 从/proc文件系统获取进程信息
func (app *Application) getProcessInfoFromProc() *ProcessInfo {
	info := &ProcessInfo{}
	
	// 读取/proc/PID/stat文件
	statFile := fmt.Sprintf("/proc/%d/stat", app.PID)
	_, err := os.ReadFile(statFile)
	if err != nil {
		return info
	}
	
	// 读取/proc/PID/status文件获取内存信息
	statusFile := fmt.Sprintf("/proc/%d/status", app.PID)
	statusData, err := os.ReadFile(statusFile)
	if err == nil {
		lines := strings.Split(string(statusData), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if rss, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						info.MemoryKB = rss
					}
				}
			}
		}
	}
	
	// 简单的CPU使用率估算（基于进程存在时间）
	// 这里可以根据需要实现更复杂的CPU使用率计算
	if !app.StartTime.IsZero() {
		uptime := time.Since(app.StartTime)
		if uptime.Minutes() > 1 {
			// 假设一个基础的CPU使用率
			info.CPUPercent = 1.0
		}
	}
	
	return info
}

// getProcessInfoWindows 在Windows系统上获取进程信息
func (app *Application) getProcessInfoWindows() *ProcessInfo {
	info := &ProcessInfo{}
	
	// 使用wmic命令获取进程信息
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", app.PID), 
		"get", "ProcessId,PageFileUsage,WorkingSetSize", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		return info
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Node") {
			continue
		}
		
		fields := strings.Split(line, ",")
		if len(fields) >= 3 {
			// 内存使用(字节转KB)
			if ws, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				info.MemoryKB = ws / 1024
			}
		}
	}
	
	return info
}

// GetCPUUsageString 获取CPU使用率字符串
func (app *Application) GetCPUUsageString() string {
	if !app.IsRunning() {
		return "0%"
	}
	
	info := app.GetProcessInfo()
	if info.CPUPercent > 100 {
		info.CPUPercent = 100
	}
	
	return fmt.Sprintf("%.1f%%", info.CPUPercent)
}

// GetMemoryUsageString 获取内存使用率字符串
func (app *Application) GetMemoryUsageString() string {
	if !app.IsRunning() {
		return "0%"
	}
	
	info := app.GetProcessInfo()
	if info.MemoryPercent > 100 {
		info.MemoryPercent = 100
	}
	
	return fmt.Sprintf("%.1f%%", info.MemoryPercent)
}

// GetMemorySizeString 获取内存大小字符串（智能单位）
func (app *Application) GetMemorySizeString() string {
	if !app.IsRunning() {
		return "0MB"
	}
	
	info := app.GetProcessInfo()
	memoryMB := float64(info.MemoryKB) / 1024
	
	if memoryMB >= 1024 {
		return fmt.Sprintf("%.1fGB", memoryMB/1024)
	} else {
		return fmt.Sprintf("%.0fMB", memoryMB)
	}
}

// GetPortString 获取应用端口信息
func (app *Application) GetPortString() string {
	if !app.IsRunning() {
		return "--"
	}
	
	// 尝试从日志文件中获取端口信息
	if app.LogFile != "" {
		if port := app.getPortFromLog(); port != "" {
			return port
		}
	}
	
	// 尝试从网络连接中获取端口信息
	if port := app.getPortFromNetstat(); port != "" {
		return port
	}
	
	return "--"
}

// getPortFromLog 从日志文件中获取端口信息
func (app *Application) getPortFromLog() string {
	file, err := os.Open(app.LogFile)
	if err != nil {
		return ""
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	
	// 简化的端口匹配模式
	portPattern := regexp.MustCompile(`(?i)(?:started|listening|listen|bind).*?port[s]?\s*[:\(]?\s*(\d+)`)
	
	for scanner.Scan() {
		line := scanner.Text()
		matches := portPattern.FindStringSubmatch(line)
		if len(matches) > 1 {
			port := matches[1]
			// 验证是否为有效端口号
			if portNum, err := strconv.Atoi(port); err == nil && portNum > 0 && portNum < 65536 {
				return port
			}
		}
	}
	
	return ""
}

// getPortFromNetstat 从网络连接中获取端口信息
func (app *Application) getPortFromNetstat() string {
	if runtime.GOOS == "windows" {
		return app.getPortFromNetstatWindows()
	} else {
		return app.getPortFromNetstatLinux()
	}
}

// getPortFromNetstatWindows 在Windows系统上获取端口信息
func (app *Application) getPortFromNetstatWindows() string {
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, fmt.Sprintf("%d", app.PID)) && strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// 提取本地地址中的端口
				localAddr := fields[1]
				if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
					port := localAddr[idx+1:]
					// 验证是否为有效端口号
					if portNum, err := strconv.Atoi(port); err == nil && portNum > 0 && portNum < 65536 {
						return port
					}
				}
			}
		}
	}
	
	return ""
}

// getPortFromNetstatLinux 在Linux系统上获取端口信息
func (app *Application) getPortFromNetstatLinux() string {
	cmd := exec.Command("netstat", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	
	lines := strings.Split(string(output), "\n")
	pidStr := fmt.Sprintf("%d/", app.PID)
	
	for _, line := range lines {
		if strings.Contains(line, pidStr) && strings.Contains(line, "LISTEN") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				// 提取本地地址中的端口
				localAddr := fields[3]
				if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
					port := localAddr[idx+1:]
					// 验证是否为有效端口号
					if portNum, err := strconv.Atoi(port); err == nil && portNum > 0 && portNum < 65536 {
						return port
					}
				}
			}
		}
	}
	
	return ""
}









// ApplicationList 应用列表类型
type ApplicationList []*Application

// ApplicationStatus 应用状态信息
type ApplicationStatus struct {
	Name        string
	FilePath    string
	PID         int
	Status      AppStatus
	CPU         string
	Memory      string
	MemSize     string
	Uptime      string
	LogFile     string
	ConfigFile  string
	CPUUsage    string
	MemoryUsage string
	MemorySize  string
	Port        string
}

// FindByName 根据名称查找应用
func (list ApplicationList) FindByName(name string) *Application {
	for _, app := range list {
		if strings.EqualFold(app.Name, name) {
			return app
		}
	}
	return nil
}

// FindByPattern 根据模式查找应用
func (list ApplicationList) FindByPattern(pattern string) ApplicationList {
	if pattern == "" {
		return list
	}
	
	var result ApplicationList
	pattern = strings.ToLower(pattern)
	
	for _, app := range list {
		if strings.Contains(strings.ToLower(app.Name), pattern) ||
			strings.Contains(strings.ToLower(filepath.Base(app.FilePath)), pattern) {
			result = append(result, app)
		}
	}
	
	return result
}

// Running 返回正在运行的应用列表
func (list ApplicationList) Running() ApplicationList {
	var result ApplicationList
	for _, app := range list {
		if app.IsRunning() {
			result = append(result, app)
		}
	}
	return result
}

// Stopped 返回已停止的应用列表
func (list ApplicationList) Stopped() ApplicationList {
	var result ApplicationList
	for _, app := range list {
		if !app.IsRunning() {
			result = append(result, app)
		}
	}
	return result
}