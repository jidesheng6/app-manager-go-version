package models

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

// IsRunning 检查应用是否正在运行（Linux专用）
func (app *Application) IsRunning() bool {
	if app.PID <= 0 {
		return false
	}
	
	// Linux系统使用信号检查
	process, err := os.FindProcess(app.PID)
	if err != nil {
		return false
	}
	
	// 发送信号0来检查进程是否存在
	err = process.Signal(os.Signal(nil))
	return err == nil
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
		// 如果StartTime为零值且进程正在运行，尝试从进程信息获取启动时间
		if app.StartTime.IsZero() {
			app.StartTime = app.getProcessStartTime()
		}
	} else {
		app.Status = StatusStopped
		app.PID = 0
		// 进程停止时重置StartTime
		app.StartTime = time.Time{}
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

// GetProcessInfo 获取进程资源使用信息（仅Linux）
func (app *Application) GetProcessInfo() *ProcessInfo {
	if !app.IsRunning() {
		return &ProcessInfo{}
	}
	
	return app.getProcessInfoLinux()
}

// getProcessInfoLinux 在Linux系统上获取进程信息
func (app *Application) getProcessInfoLinux() *ProcessInfo {
	info := &ProcessInfo{}
	
	// 首先验证进程是否存在
	if app.PID <= 0 {
		return info
	}
	
	// 使用ps命令获取进程信息，增加更多字段和超时控制
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(app.PID), "-o", "pid,pcpu,pmem,rss,vsz", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		// 如果ps命令失败，尝试从/proc读取
		procInfo := app.getProcessInfoFromProc()
		// 如果/proc也没有获取到有效信息，可能进程已经不存在
		if procInfo.MemoryKB == 0 && procInfo.CPUPercent == 0 {
			// 进程可能已经停止，更新状态
			app.Status = StatusStopped
			app.PID = 0
		}
		return procInfo
	}
	
	line := strings.TrimSpace(string(output))
	if line == "" {
		// ps命令没有返回结果，进程可能已经停止
		app.Status = StatusStopped
		app.PID = 0
		return info
	}
	
	fields := strings.Fields(line)
	if len(fields) >= 4 {
		// CPU使用率
		if cpu, err := strconv.ParseFloat(fields[1], 64); err == nil {
			info.CPUPercent = cpu
			// 限制CPU使用率在合理范围内
			if info.CPUPercent > 100 {
				info.CPUPercent = 100
			}
		}
		
		// 内存使用率
		if mem, err := strconv.ParseFloat(fields[2], 64); err == nil {
			info.MemoryPercent = mem
			// 限制内存使用率在合理范围内
			if info.MemoryPercent > 100 {
				info.MemoryPercent = 100
			}
		}
		
		// 内存大小(KB)
		if rss, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
			info.MemoryKB = rss
		}
	} else {
		// ps命令返回格式异常，回退到/proc方式
		return app.getProcessInfoFromProc()
	}
	
	return info
}

// getProcessInfoFromProc 从/proc文件系统获取进程信息
func (app *Application) getProcessInfoFromProc() *ProcessInfo {
	info := &ProcessInfo{}
	
	// 读取/proc/PID/stat文件获取CPU信息
	statFile := fmt.Sprintf("/proc/%d/stat", app.PID)
	statData, err := os.ReadFile(statFile)
	if err != nil {
		return info
	}
	
	// 解析stat文件
	statFields := strings.Fields(string(statData))
	if len(statFields) >= 24 {
		// 获取用户态和内核态CPU时间
		utime, _ := strconv.ParseInt(statFields[13], 10, 64)
		stime, _ := strconv.ParseInt(statFields[14], 10, 64)
		
		// 获取系统总CPU时间用于计算CPU使用率
		cpuData, err := os.ReadFile("/proc/stat")
		if err == nil {
			cpuLines := strings.Split(string(cpuData), "\n")
			if len(cpuLines) > 0 && strings.HasPrefix(cpuLines[0], "cpu ") {
				cpuFields := strings.Fields(cpuLines[0])
				if len(cpuFields) >= 8 {
					var totalCPU int64
					for i := 1; i < len(cpuFields); i++ {
						val, _ := strconv.ParseInt(cpuFields[i], 10, 64)
						totalCPU += val
					}
					
					// 计算进程CPU使用率
					processCPU := utime + stime
					if totalCPU > 0 {
						info.CPUPercent = float64(processCPU) / float64(totalCPU) * 100.0
						// 限制CPU使用率在合理范围内
						if info.CPUPercent > 100 {
							info.CPUPercent = 100
						}
					}
				}
			}
		}
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
			} else if strings.HasPrefix(line, "VmSize:") {
				// 获取虚拟内存大小用于计算内存使用率
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if vmSize, err := strconv.ParseInt(fields[1], 10, 64); err == nil && vmSize > 0 {
						info.MemoryPercent = float64(info.MemoryKB) / float64(vmSize) * 100.0
						if info.MemoryPercent > 100 {
							info.MemoryPercent = 100
						}
					}
				}
			}
		}
		
		// 如果无法从VmSize计算内存使用率，使用系统总内存
		if info.MemoryPercent == 0 && info.MemoryKB > 0 {
			meminfoData, err := os.ReadFile("/proc/meminfo")
			if err == nil {
				meminfoLines := strings.Split(string(meminfoData), "\n")
				for _, line := range meminfoLines {
					if strings.HasPrefix(line, "MemTotal:") {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							if totalMem, err := strconv.ParseInt(fields[1], 10, 64); err == nil && totalMem > 0 {
								info.MemoryPercent = float64(info.MemoryKB) / float64(totalMem) * 100.0
								if info.MemoryPercent > 100 {
									info.MemoryPercent = 100
								}
							}
						}
						break
					}
				}
			}
		}
	}
	
	// 如果仍然没有获取到有效的CPU使用率，提供一个基础估算
	if info.CPUPercent == 0 && !app.StartTime.IsZero() {
		uptime := time.Since(app.StartTime)
		if uptime.Minutes() > 1 {
			// 基于运行时间的简单估算
			info.CPUPercent = 0.5
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

// GetPortsString 获取应用监听的端口列表（仅Linux）
func (app *Application) GetPortsString() string {
	if !app.IsRunning() {
		return "--"
	}
	
	ports := app.getListeningPorts()
	if len(ports) == 0 {
		return "--"
	}
	
	// 如果只有一个端口，直接返回
	if len(ports) == 1 {
		return ports[0]
	}
	
	// 多个端口用逗号分隔，最多显示3个
	if len(ports) > 3 {
		return fmt.Sprintf("%s,+%d", strings.Join(ports[:3], ","), len(ports)-3)
	}
	
	return strings.Join(ports, ",")
}

// getListeningPorts 获取进程监听的端口列表（仅Linux）
func (app *Application) getListeningPorts() []string {
	if app.PID <= 0 {
		return nil
	}
	
	var ports []string
	
	// 首先尝试使用ss命令（更现代的替代方案）
	ports = app.getPortsWithSS()
	if len(ports) > 0 {
		return ports
	}
	
	// 如果ss失败，使用netstat命令获取端口信息
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "netstat", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		// 如果netstat也失败，尝试从/proc/net/tcp读取
		ports = app.getPortsFromProcNet()
		if len(ports) == 0 {
			// 最后尝试从日志文件中获取
			ports = app.getPortsFromLog()
		}
		return ports
	}
	
	lines := strings.Split(string(output), "\n")
	pidStr := fmt.Sprintf("%d/", app.PID)
	
	for _, line := range lines {
		if strings.Contains(line, pidStr) && strings.Contains(line, "LISTEN") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				// 解析地址:端口格式
				addr := fields[3]
				if strings.Contains(addr, ":") {
					parts := strings.Split(addr, ":")
					if len(parts) >= 2 {
						port := parts[len(parts)-1]
						// 验证端口号
						if portNum, err := strconv.Atoi(port); err == nil && portNum > 0 && portNum <= 65535 {
							// 避免重复端口
							found := false
							for _, p := range ports {
								if p == port {
									found = true
									break
								}
							}
							if !found {
								ports = append(ports, port)
							}
						}
					}
				}
			}
		}
	}
	
	// 如果netstat没有找到端口，尝试从日志文件中获取
	if len(ports) == 0 {
		logPorts := app.getPortsFromLog()
		ports = append(ports, logPorts...)
	}
	
	return ports
}

// getPortsFromLog 从日志文件中获取端口信息
func (app *Application) getPortsFromLog() []string {
	if app.LogFile == "" {
		return nil
	}
	
	file, err := os.Open(app.LogFile)
	if err != nil {
		return nil
	}
	defer file.Close()
	
	var ports []string
	scanner := bufio.NewScanner(file)
	
	// 常见的端口模式
	portPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:started|listening|running).*?(?:on|port).*?(\d{4,5})`),
		regexp.MustCompile(`(?i)tomcat.*?started.*?port.*?(\d{4,5})`),
		regexp.MustCompile(`(?i)jetty.*?started.*?port.*?(\d{4,5})`),
		regexp.MustCompile(`(?i)undertow.*?started.*?port.*?(\d{4,5})`),
		regexp.MustCompile(`(?i)netty.*?started.*?port.*?(\d{4,5})`),
		regexp.MustCompile(`(?i)server.*?port.*?(\d{4,5})`),
		regexp.MustCompile(`(?i)listening.*?(\d{4,5})`),
		regexp.MustCompile(`(?i)port.*?(\d{4,5})`),
	}
	
	for scanner.Scan() {
		line := scanner.Text()
		for _, pattern := range portPatterns {
			matches := pattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) > 1 {
					port := match[1]
					// 验证端口范围
					if portNum, err := strconv.Atoi(port); err == nil && portNum >= 1024 && portNum <= 65535 {
						// 避免重复端口
						found := false
						for _, p := range ports {
							if p == port {
								found = true
								break
							}
						}
						if !found {
							ports = append(ports, port)
						}
					}
				}
			}
		}
	}
	
	return ports
}

// getProcessStartTime 从进程信息获取启动时间
func (app *Application) getProcessStartTime() time.Time {
	if app.PID <= 0 {
		return time.Time{}
	}
	
	// 尝试从/proc/PID/stat获取进程启动时间
	statFile := fmt.Sprintf("/proc/%d/stat", app.PID)
	data, err := os.ReadFile(statFile)
	if err != nil {
		return time.Time{}
	}
	
	fields := strings.Fields(string(data))
	if len(fields) < 22 {
		return time.Time{}
	}
	
	// 第22个字段是进程启动时间（以时钟滴答为单位）
	starttime, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return time.Time{}
	}
	
	// 获取系统启动时间
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}
	}
	
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) < 1 {
		return time.Time{}
	}
	
	uptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return time.Time{}
	}
	
	// 计算进程启动时间
	// 时钟滴答频率通常是100Hz
	clockTicks := int64(100)
	processStartSeconds := float64(starttime) / float64(clockTicks)
	systemBootTime := time.Now().Add(-time.Duration(uptime * float64(time.Second)))
	processStartTime := systemBootTime.Add(time.Duration(processStartSeconds * float64(time.Second)))
	
	return processStartTime
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
	Port        string
	LogFile     string
	ConfigFile  string
	CPUUsage    string
	MemoryUsage string
	MemorySize  string
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