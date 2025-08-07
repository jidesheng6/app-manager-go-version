package models

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Application 应用程序模型
type Application struct {
	Name       string    // 应用名称
	FilePath   string    // JAR/WAR文件路径
	PID        int       // 进程ID
	Status     AppStatus // 运行状态
	LogFile    string    // 日志文件路径
	PIDFile    string    // PID文件路径
	ConfigFile string    // 配置文件路径
	Ports      string    // 应用监听的端口
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

	// 在Linux上，检查 /proc/[PID] 目录是否存在是更可靠的方法
	procPath := fmt.Sprintf("/proc/%d", app.PID)
	if _, err := os.Stat(procPath); err == nil {
		// 目录存在，进程正在运行
		return true
	}

	return false
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
	}
}

// GetUptimeString 获取运行时长的字符串表示
func (app *Application) GetUptimeString() string {
	info := app.GetProcessInfo()
	if !app.IsRunning() || info.Etimes == 0 {
		return "--"
	}

	d := time.Duration(info.Etimes) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 { // 对于小于1分钟的情况，显示秒
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	return strings.Join(parts, " ")
}

// ProcessInfo 进程信息结构
type ProcessInfo struct {
	CPUPercent    float64
	MemoryKB      int64
	MemoryPercent float64
	Etimes        int64 // 进程已运行秒数
}

// GetProcessInfo 获取进程资源使用信息
func (app *Application) GetProcessInfo() *ProcessInfo {
	if !app.IsRunning() {
		return &ProcessInfo{}
	}
	// 从批量获取的信息中查找
	if info, ok := BatchProcessInfo[app.PID]; ok {
		return info
	}
	// 如果批量信息中没有，则单独获取（作为后备）
	return app.getProcessInfoLinux()
}

// getProcessInfoLinux 在Linux系统上获取进程信息
func (app *Application) getProcessInfoLinux() *ProcessInfo {
	info := &ProcessInfo{}

	// 使用ps命令获取进程信息，增加 etimes
	cmd := exec.Command("ps", "-p", strconv.Itoa(app.PID), "-o", "pcpu,pmem,rss,etimes", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		// 如果ps命令失败，则不返回任何信息
		return info
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return info
	}

	fields := strings.Fields(line)
	if len(fields) >= 4 {
		// CPU使用率
		if cpu, err := strconv.ParseFloat(fields[0], 64); err == nil {
			info.CPUPercent = cpu
		}

		// 内存使用率
		if mem, err := strconv.ParseFloat(fields[1], 64); err == nil {
			info.MemoryPercent = mem
		}

		// 内存大小(KB)
		if rss, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
			info.MemoryKB = rss
		}

		// 运行时长（秒）
		if etimes, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
			info.Etimes = etimes
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
	if !app.IsRunning() || app.GetProcessInfo().MemoryKB == 0 {
		return "0B"
	}
	
	kiloBytes := float64(app.GetProcessInfo().MemoryKB)
	
	if kiloBytes < 1024 {
		return fmt.Sprintf("%.0fK", kiloBytes)
	}
	
	megaBytes := kiloBytes / 1024
	if megaBytes < 1024 {
		return fmt.Sprintf("%.1fM", megaBytes)
	}
	
	gigaBytes := megaBytes / 1024
	if gigaBytes < 1024 {
		return fmt.Sprintf("%.1fG", gigaBytes)
	}
	
	teraBytes := gigaBytes / 1024
	return fmt.Sprintf("%.1fT", teraBytes)
}


// BatchProcessInfo 包含所有进程信息的映射
var BatchProcessInfo = make(map[int]*ProcessInfo)

// BatchUpdateProcessInfo 批量更新所有应用的进程信息
func BatchUpdateProcessInfo(apps ApplicationList) {
	// 清空旧数据
	BatchProcessInfo = make(map[int]*ProcessInfo)

	// 一次性执行ps命令
	cmd := exec.Command("ps", "-eo", "pid,pcpu,pmem,rss,etimes", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return // ps命令失败则不更新
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		info := &ProcessInfo{}
		if cpu, err := strconv.ParseFloat(fields[1], 64); err == nil {
			info.CPUPercent = cpu
		}
		if mem, err := strconv.ParseFloat(fields[2], 64); err == nil {
			info.MemoryPercent = mem
		}
		if rss, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
			info.MemoryKB = rss
		}
		if etimes, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
			info.Etimes = etimes
		}

		BatchProcessInfo[pid] = info
	}
}


// ApplicationList 应用列表类型
type ApplicationList []*Application

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