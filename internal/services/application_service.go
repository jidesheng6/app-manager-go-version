package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ApplicationService 应用服务
type ApplicationService struct {
	config        *config.Config
	sampleManager *SampleManager
}

// NewApplicationService 创建应用服务实例
func NewApplicationService() *ApplicationService {
	return &ApplicationService{
		config:        config.AppConfig,
		sampleManager: NewSampleManager(),
	}
}

// DiscoverApplications 发现应用程序
func (s *ApplicationService) DiscoverApplications() (models.ApplicationList, error) {
	backendDir := config.GetBackendDir()

	var applications models.ApplicationList

	// 查找JAR和WAR文件
	patterns := []string{"*.jar", "*.war"}
	if s.config.Application.AppType == "jar" {
		patterns = []string{"*.jar"}
	} else if s.config.Application.AppType == "war" {
		patterns = []string{"*.war"}
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(backendDir, pattern))
		if err != nil {
			continue
		}

		for _, match := range matches {
			app := models.NewApplication(match)

			// 设置日志文件路径
			app.LogFile = filepath.Join(config.GetLogsDir(), app.Name+".log")

			// 从配置中加载应用的优先级和延迟
			if priorityConfig, ok := s.config.Applications[app.Name]; ok {
				app.Priority = priorityConfig.Priority
				app.StartupDelay = priorityConfig.StartupDelay
			} else {
				// 如果未配置，则使用默认优先级
				app.Priority = s.config.Application.DefaultPriority
				if app.Priority == 0 {
					app.Priority = 999 // 兜底默认值
				}
				app.StartupDelay = s.config.Application.StartupDelay
			}

			// 加载PID信息
			pidFile := filepath.Join(config.GetPidsDir(), app.Name+".pid")
			if err := app.LoadPID(pidFile); err != nil {
				logger.Warning("加载PID文件失败 %s: %v", app.Name, err)
			}

			// 更新状态
			app.UpdateStatus()

			// 查找配置文件
			configFile := s.findConfigFile(app.Name)
			if configFile != "" {
				app.ConfigFile = configFile
			}

			applications = append(applications, app)
		}
	}

	return applications, nil
}

// StartApplication 启动应用
func (s *ApplicationService) StartApplication(app *models.Application) error {
	// 启动前检查：如果PID文件无效，则清理
	if app.PID > 0 && !app.IsRunning() {
		logger.Warning("发现陈旧的PID文件: %s，已自动清理", app.PIDFile)
		app.RemovePID()
	}

	if app.IsRunning() {
		return fmt.Errorf("应用 %s 已经在运行 (PID: %d)", app.Name, app.PID)
	}

	logger.Info("正在启动应用: %s", app.Name)

	// 构建启动命令
	args := []string{}

	// 获取优化后的JVM参数（包含随机数优化）
	jvmParams := config.GetOptimizedJVMParams()
	if jvmParams != "" {
		jvmArgs := strings.Fields(jvmParams)
		args = append(args, jvmArgs...)
	}

	// 添加配置文件参数
	if app.ConfigFile != "" {
		// 检查配置文件是否存在
		if _, err := os.Stat(app.ConfigFile); err == nil {
			// 根据application_config_type选择配置方式
			switch s.config.Application.ApplicationConfigType {
			case "spring_cloud":
				// Spring Cloud模式：使用标准的bootstrap配置文件
				// Spring Cloud会自动寻找bootstrap.yml，这里只是确保路径正确
				logger.Info("使用Spring Cloud配置文件: %s", filepath.Base(app.ConfigFile))
				logger.Info("Spring Cloud将自动加载bootstrap配置")
			case "spring_local":
				fallthrough
			default:
				// Spring Local模式：使用传统的config.location
				args = append(args, "-Dspring.config.location=file:"+app.ConfigFile)
				logger.Info("使用本地配置文件: %s", filepath.Base(app.ConfigFile))
			}
		} else {
			logger.Warning("配置文件不存在，跳过配置文件参数: %s", app.ConfigFile)
		}
	} else {
		// 没有找到特定的配置文件
		switch s.config.Application.ApplicationConfigType {
		case "spring_cloud":
			// Spring Cloud模式：让Spring Cloud自动寻找bootstrap配置
			logger.Info("Spring Cloud模式：将自动搜索bootstrap配置文件")
		case "spring_local":
			fallthrough
		default:
			// Spring Local模式不需要额外的配置目录参数，Spring Boot会自动搜索
			logger.Info("使用Spring Boot默认配置搜索路径")
		}
		logger.Info("使用应用内部默认配置，配置类型: %s", s.config.Application.ApplicationConfigType)
	}

	// 添加JAR文件
	args = append(args, "-jar", app.FilePath)

	// 创建日志目录
	if err := os.MkdirAll(filepath.Dir(app.LogFile), 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 创建日志文件
	logFile, err := os.OpenFile(app.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logFile.Close()

	// 创建命令
	cmd := exec.Command(s.config.Global.JavaPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// 设置环境变量
	cmd.Env = os.Environ()
	// 设置进程组，使Java进程独立于父进程信号（仅Linux）
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// 添加Nacos环境变量（如果启用）
	if config.IsNacosEnabled() {
		nacosEnvVars := config.GetNacosEnvironmentVars()

		// 显示Nacos配置信息
		logger.Info("🔧 Nacos配置:")
		for key, value := range nacosEnvVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
			logger.Info("   %s: %s", key, value)
		}
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动应用失败: %w", err)
	}

	// 保存PID
	app.PID = cmd.Process.Pid
	app.Status = models.StatusStarting

	pidFile := filepath.Join(config.GetPidsDir(), app.Name+".pid")
	if err := app.SavePID(pidFile); err != nil {
		logger.Warning("保存PID文件失败: %v", err)
	}

	logger.Info("应用启动中，PID: %d", app.PID)
	logger.Info("日志文件: %s", app.LogFile)

	// 设置为运行状态
	app.Status = models.StatusRunning
	logger.Success("应用启动命令已执行，PID: %d", app.PID)
	logger.Info("使用 '%s status' 查看应用状态", config.GetProgramName())

	return nil
}

// StopApplication 停止应用
func (s *ApplicationService) StopApplication(app *models.Application) error {
	logger.Info("正在准备强制停止应用: %s", app.Name)

	// 1. 合并来自PID文件和系统进程列表的PID
	pidsToStop := make(map[int]bool)
	if app.PID > 0 && app.IsRunning() {
		pidsToStop[app.PID] = true
	}

	orphanPIDs, err := s.findPIDsByAppName(app.Name)
	if err != nil {
		logger.Warning("搜索孤儿进程失败: %v", err)
	}
	for _, pid := range orphanPIDs {
		pidsToStop[pid] = true
	}

	if len(pidsToStop) == 0 {
		logger.Info("应用 %s 未发现正在运行的进程。", app.Name)
		app.RemovePID() // 确保清理无效的PID文件
		app.Status = models.StatusStopped
		return nil
	}

	// 2. 强制停止所有找到的进程
	logger.Info("将强制终止以下进程: %v", pidsToStop)
	for pid := range pidsToStop {
		process, err := os.FindProcess(pid)
		if err != nil {
			logger.Warning("查找进程 PID %d 失败: %v", pid, err)
			continue
		}
		if err := process.Kill(); err != nil {
			logger.Error("强制终止 PID %d 失败: %v", pid, err)
		}
	}

	// 3. 最终清理
	app.RemovePID()
	app.Status = models.StatusStopped
	logger.Success("应用 %s 已被强制停止。", app.Name)
	return nil
}

// RestartApplication 重启应用
func (s *ApplicationService) RestartApplication(app *models.Application) error {
	logger.Info("正在重启应用: %s", app.Name)

	// 先停止
	if app.IsRunning() {
		if err := s.StopApplication(app); err != nil {
			return fmt.Errorf("停止应用失败: %w", err)
		}

		// 等待完全停止
		for i := 0; i < 5; i++ {
			if !app.IsRunning() {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	// 再启动
	return s.StartApplication(app)
}

// GetApplicationStatus 获取应用状态信息
func (s *ApplicationService) GetApplicationStatus(app *models.Application) *models.ApplicationStatus {
	// 更新应用的实际状态
	s.updateApplicationRealStatus(app)

	// 如果应用正在运行，获取端口信息
	if app.Status == models.StatusRunning {
		logger.Info("[调试] 步骤2/2: 正在为应用 '%s' (PID: %d) 获取端口信息...", app.Name, app.PID)
		app.Ports = s.getPortsForPID(app.PID)
	} else {
		app.Ports = "" // 清空端口信息
	}

	return &models.ApplicationStatus{
		Name:        app.Name,
		FilePath:    app.FilePath,
		PID:         app.PID,
		Status:      app.Status,
		Uptime:      app.GetUptimeString(),
		LogFile:     app.LogFile,
		ConfigFile:  app.ConfigFile,
		CPUUsage:    app.GetCPUUsageString(),
		MemoryUsage: app.GetMemoryUsageString(),
		MemorySize:  app.GetMemorySizeString(),
		Ports:       app.Ports,
	}
}

// updateApplicationRealStatus 更新应用的真实状态
func (s *ApplicationService) updateApplicationRealStatus(app *models.Application) {
	if app.PID > 0 {
		if !app.IsRunning() {
			// PID文件存在，但进程不存在
			logger.Warning("发现无效的PID文件: %s (PID: %d)", app.PIDFile, app.PID)
			app.Status = models.StatusStopped
			app.RemovePID() // 清理无效的PID文件
		} else {
			// 进程存在
			app.Status = models.StatusRunning
		}
	} else {
		// 没有PID文件
		app.Status = models.StatusStopped
	}
}

// getPortsForPID 获取指定PID占用的TCP端口
func (s *ApplicationService) getPortsForPID(pid int) string {
	if pid <= 0 {
		return ""
	}

	// 使用 ss 命令查找端口
	// ss -lntp: -l(listening), -n(numeric), -t(tcp), -p(processes)
	cmd := exec.Command("ss", "-lntp")
	output, err := cmd.Output()
	if err != nil {
		logger.Warning("执行 'ss -lntp' 失败: %v", err)
		return ""
	}

	var ports []string
	pidStr := fmt.Sprintf("pid=%d", pid)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.Contains(line, pidStr) {
			// 示例输出: LISTEN 0 4096 127.0.0.1:8080 *:* users:(("java",pid=123,fd=4))
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				// 提取地址和端口
				addrPort := fields[3]
				// 从地址中提取端口
				parts := strings.Split(addrPort, ":")
				if len(parts) > 0 {
					port := parts[len(parts)-1]
					ports = append(ports, port)
				}
			}
		}
	}

	return strings.Join(ports, ",")
}

// getPortConnectionsForPID 获取指定PID的端口连接状态详细信息
func (s *ApplicationService) getPortConnectionsForPID(pid int) string {
	if pid <= 0 {
		return ""
	}

	// 使用 ss 命令查找所有连接状态
	// ss -antp: -a(all), -n(numeric), -t(tcp), -p(processes)
	cmd := exec.Command("ss", "-antp")
	output, err := cmd.Output()
	if err != nil {
		logger.Warning("执行 'ss -antp' 失败: %v", err)
		return ""
	}

	var connections []string
	pidStr := fmt.Sprintf("pid=%d", pid)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.Contains(line, pidStr) {
			// 示例输出: ESTAB 0 0 192.168.1.100:8080 192.168.1.200:45678 users:(("java",pid=123,fd=4))
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				state := fields[0]
				localAddr := fields[3]
				remoteAddr := fields[4]

				// 提取本地端口
				localParts := strings.Split(localAddr, ":")
				if len(localParts) > 0 {
					localPort := localParts[len(localParts)-1]
					connectionInfo := fmt.Sprintf("%s:%s(%s)", localPort, remoteAddr, state)
					connections = append(connections, connectionInfo)
				}
			}
		}
	}

	return strings.Join(connections, ";")
}

// getDiskIOForPID 获取指定PID的磁盘I/O信息
func (s *ApplicationService) getDiskIOForPID(pid int) (readBytes, writeBytes uint64, err error) {
	if pid <= 0 {
		return 0, 0, fmt.Errorf("invalid PID: %d", pid)
	}

	ioFile := fmt.Sprintf("/proc/%d/io", pid)
	data, err := os.ReadFile(ioFile)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read %s: %w", ioFile, err)
	}

	return s.parseIOStats(string(data))
}

// parseIOStats 解析 /proc/[PID]/io 文件内容
func (s *ApplicationService) parseIOStats(content string) (readBytes, writeBytes uint64, err error) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		key := fields[0]
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}

		switch key {
		case "read_bytes:":
			readBytes = value
		case "write_bytes:":
			writeBytes = value
		}
	}
	return readBytes, writeBytes, nil
}

// calculateDiskIORate 计算磁盘I/O速率
func (s *ApplicationService) calculateDiskIORate(pid int, currentRead, currentWrite uint64) (readRate, writeRate float64) {
	// 使用采样管理器计算速率
	readRate, writeRate, _, _ = s.sampleManager.GetRate(pid, currentRead, currentWrite, 0, 0)
	return readRate, writeRate
}

// getNetworkStats 获取系统网络统计信息
func (s *ApplicationService) getNetworkStats() (rxBytes, txBytes uint64, err error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read /proc/net/dev: %w", err)
	}

	return s.parseNetworkStats(string(data))
}

// parseNetworkStats 解析 /proc/net/dev 文件内容
func (s *ApplicationService) parseNetworkStats(content string) (rxBytes, txBytes uint64, err error) {
	lines := strings.Split(content, "\n")
	var totalRx, totalTx uint64

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "Inter-") && !strings.HasPrefix(line, "face") {
			// 解析网络接口行，格式如: eth0: 1234 5678 ...
			parts := strings.Split(line, ":")
			if len(parts) != 2 {
				continue
			}

			interfaceName := strings.TrimSpace(parts[0])
			// 跳过回环接口
			if interfaceName == "lo" {
				continue
			}

			fields := strings.Fields(strings.TrimSpace(parts[1]))
			if len(fields) >= 9 {
				// 字段顺序: bytes packets errs drop fifo frame compressed multicast bytes packets errs drop fifo colls carrier compressed
				// 索引0: 接收字节数, 索引8: 发送字节数
				if rxBytesInterface, parseErr := strconv.ParseUint(fields[0], 10, 64); parseErr == nil {
					totalRx += rxBytesInterface
				}
				if txBytesInterface, parseErr := strconv.ParseUint(fields[8], 10, 64); parseErr == nil {
					totalTx += txBytesInterface
				}
			}
		}
	}

	return totalRx, totalTx, nil
}

// getNetworkStatsForPID 获取指定进程的网络统计信息
// 注意：由于Linux系统限制，进程级网络统计难以精确获取，此方法返回系统级网络统计作为近似值
func (s *ApplicationService) getNetworkStatsForPID(pid int) (rxBytes, txBytes uint64, err error) {
	// 直接使用系统级网络统计作为回退策略
	// 这是一个简化的实现，避免了复杂的进程级网络统计获取
	return s.getNetworkStats()
}

// getNetworkStatsFromConnections 通过网络连接获取进程网络统计（更精确的方法）
func (s *ApplicationService) getNetworkStatsFromConnections(pid int) (rxBytes, txBytes uint64, err error) {
	var totalRx, totalTx uint64

	// 获取进程的socket文件描述符
	socketInodes, err := s.getProcessSocketInodes(pid)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get socket inodes for PID %d: %v", pid, err)
	}

	if len(socketInodes) == 0 {
		// 进程没有网络连接，返回0
		return 0, 0, nil
	}

	// 解析TCP连接
	tcpRx, tcpTx, err := s.getNetworkStatsFromProtocol("tcp", socketInodes)
	if err == nil {
		totalRx += tcpRx
		totalTx += tcpTx
	}

	// 解析UDP连接
	udpRx, udpTx, err := s.getNetworkStatsFromProtocol("udp", socketInodes)
	if err == nil {
		totalRx += udpRx
		totalTx += udpTx
	}

	return totalRx, totalTx, nil
}

// getProcessSocketInodes 获取进程的所有socket inode
func (s *ApplicationService) getProcessSocketInodes(pid int) (map[string]bool, error) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, err
	}

	socketInodes := make(map[string]bool)
	for _, entry := range entries {
		fdPath := filepath.Join(fdDir, entry.Name())
		linkTarget, err := os.Readlink(fdPath)
		if err != nil {
			continue
		}

		// 检查是否是socket链接 (格式: socket:[inode])
		if strings.HasPrefix(linkTarget, "socket:[") && strings.HasSuffix(linkTarget, "]") {
			inode := linkTarget[8 : len(linkTarget)-1] // 提取inode号
			socketInodes[inode] = true
		}
	}

	return socketInodes, nil
}

// getNetworkStatsFromProtocol 从指定协议文件中获取网络统计
func (s *ApplicationService) getNetworkStatsFromProtocol(protocol string, socketInodes map[string]bool) (uint64, uint64, error) {
	procFile := fmt.Sprintf("/proc/net/%s", protocol)
	file, err := os.Open(procFile)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(file)

	// 跳过标题行
	if scanner.Scan() {
		// 标题行，跳过
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// /proc/net/tcp 和 /proc/net/udp 的格式：
		// sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode
		if len(fields) < 10 {
			continue
		}

		inode := fields[9]
		if socketInodes[inode] {
			// 这个连接属于我们的进程
			// 注意：/proc/net/tcp 中的 tx_queue 和 rx_queue 是队列中的字节数，不是总传输量
			// 这里我们只能获取当前队列状态，无法获取历史传输总量

			// 解析 tx_queue (发送队列)
			if txQueue, err := strconv.ParseUint(fields[4], 16, 64); err == nil {
				totalTx += txQueue
			}

			// 解析 rx_queue (接收队列)
			if rxQueue, err := strconv.ParseUint(fields[5], 16, 64); err == nil {
				totalRx += rxQueue
			}
		}
	}

	return totalRx, totalTx, scanner.Err()
}

// calculateNetworkRate 计算网络传输速率
func (s *ApplicationService) calculateNetworkRate(pid int, currentRx, currentTx uint64) (rxRate, txRate float64) {
	// 使用采样管理器计算速率
	_, _, rxRate, txRate = s.sampleManager.GetRate(pid, 0, 0, currentRx, currentTx)
	return rxRate, txRate
}

// IOSample 表示一个I/O采样点
type IOSample struct {
	Timestamp  time.Time
	ReadBytes  uint64
	WriteBytes uint64
	NetworkRx  uint64
	NetworkTx  uint64
}

// SampleManager 管理采样数据
type SampleManager struct {
	samples map[int][]IOSample // PID -> 采样历史
	mutex   sync.RWMutex
}

// NewSampleManager 创建新的采样管理器
func NewSampleManager() *SampleManager {
	return &SampleManager{
		samples: make(map[int][]IOSample),
	}
}

// AddSample 添加新的采样点
func (sm *SampleManager) AddSample(pid int, sample IOSample) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	samples := sm.samples[pid]
	samples = append(samples, sample)

	// 保持最近10个采样点
	if len(samples) > 10 {
		samples = samples[len(samples)-10:]
	}

	sm.samples[pid] = samples
}

// GetRate 计算速率（字节/秒）
func (sm *SampleManager) GetRate(pid int, currentRead, currentWrite, currentRx, currentTx uint64) (readRate, writeRate, rxRate, txRate float64) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	samples := sm.samples[pid]
	if len(samples) < 2 {
		return 0, 0, 0, 0
	}

	// 使用最近两个采样点计算速率
	prev := samples[len(samples)-1]
	now := time.Now()

	timeDiff := now.Sub(prev.Timestamp).Seconds()
	if timeDiff <= 0 {
		return 0, 0, 0, 0
	}

	readRate = float64(currentRead-prev.ReadBytes) / timeDiff
	writeRate = float64(currentWrite-prev.WriteBytes) / timeDiff
	rxRate = float64(currentRx-prev.NetworkRx) / timeDiff
	txRate = float64(currentTx-prev.NetworkTx) / timeDiff

	return readRate, writeRate, rxRate, txRate
}

// RemovePID 移除PID的采样数据
func (sm *SampleManager) RemovePID(pid int) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	delete(sm.samples, pid)
}

// formatBytes 格式化字节数为人类可读格式
func (s *ApplicationService) formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatRate 格式化速率为人类可读格式
func (s *ApplicationService) formatRate(bytesPerSec float64) string {
	if bytesPerSec < 0 {
		bytesPerSec = 0
	}
	return s.formatBytes(uint64(bytesPerSec)) + "/s"
}

// BatchUpdateProcessInfoWithMonitoring 批量更新所有应用的进程信息（包含监控数据）
func (s *ApplicationService) BatchUpdateProcessInfoWithMonitoring(apps models.ApplicationList) {
	// 首先调用原有的批量更新
	models.BatchUpdateProcessInfo(apps)

	// 如果监控功能未启用，直接返回
	if !s.config.Monitoring.EnableDiskIO && !s.config.Monitoring.EnableNetwork && !s.config.Monitoring.EnablePortStats {
		return
	}

	// 为每个运行中的应用收集监控数据
	for _, app := range apps {
		if !app.IsRunning() {
			// 清理已停止应用的采样数据
			s.sampleManager.RemovePID(app.PID)
			continue
		}

		// 获取进程信息
		processInfo := models.BatchProcessInfo[app.PID]
		if processInfo == nil {
			continue
		}

		// 收集磁盘I/O数据
		if s.config.Monitoring.EnableDiskIO {
			readBytes, writeBytes, err := s.getDiskIOForPID(app.PID)
			if err == nil {
				processInfo.DiskReadBytes = readBytes
				processInfo.DiskWriteBytes = writeBytes

				// 计算磁盘I/O速率
				readRate, writeRate, _, _ := s.sampleManager.GetRate(app.PID, readBytes, writeBytes, 0, 0)
				processInfo.DiskReadRate = s.formatRate(readRate)
				processInfo.DiskWriteRate = s.formatRate(writeRate)
			}
		}

		// 收集进程级网络数据
		if s.config.Monitoring.EnableNetwork {
			// 为每个进程获取独立的网络统计
			processRx, processTx, err := s.getNetworkStatsForPID(app.PID)
			if err == nil {
				processInfo.NetworkRxBytes = processRx
				processInfo.NetworkTxBytes = processTx
				processInfo.NetworkDataSource = "system-level" // 标识为系统级数据

				// 计算进程级网络速率
				rxRate, txRate := s.calculateNetworkRate(app.PID, processRx, processTx)
				processInfo.NetworkRxRate = s.formatRate(rxRate)
				processInfo.NetworkTxRate = s.formatRate(txRate)
			} else {
				// 如果获取进程级网络统计失败，回退到系统级统计
				systemRx, systemTx, sysErr := s.getNetworkStats()
				if sysErr == nil {
					processInfo.NetworkRxBytes = systemRx
					processInfo.NetworkTxBytes = systemTx
					processInfo.NetworkDataSource = "system-level" // 标识为系统级数据

					// 使用系统级数据计算速率（但每个进程会显示相同值）
					rxRate, txRate := s.calculateNetworkRate(app.PID, systemRx, systemTx)
					processInfo.NetworkRxRate = s.formatRate(rxRate)
					processInfo.NetworkTxRate = s.formatRate(txRate)
				} else {
					processInfo.NetworkDataSource = "unavailable" // 标识为不可用
				}
			}
		}

		// 收集端口连接状态
		if s.config.Monitoring.EnablePortStats {
			processInfo.PortConnections = s.getPortConnectionsForPID(app.PID)
		}

		// 添加采样点
		sample := IOSample{
			Timestamp:  time.Now(),
			ReadBytes:  processInfo.DiskReadBytes,
			WriteBytes: processInfo.DiskWriteBytes,
			NetworkRx:  processInfo.NetworkRxBytes,
			NetworkTx:  processInfo.NetworkTxBytes,
		}
		s.sampleManager.AddSample(app.PID, sample)
	}
}

// findPIDsByAppName 通过应用名称查找所有相关Java进程的PID
func (s *ApplicationService) findPIDsByAppName(appName string) ([]int, error) {
	var pids []int
	// 使用 pgrep 命令可以更直接地查找
	// pgrep -f "java.*-jar.*party-auth.jar"
	pattern := fmt.Sprintf("java.*-jar.*%s.jar", appName)
	cmd := exec.Command("pgrep", "-f", pattern)
	output, err := cmd.Output()
	if err != nil {
		// pgrep 在没有找到匹配项时会返回非0退出码
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return pids, nil // 没有找到进程，不是一个真正的错误
		}
		return nil, fmt.Errorf("执行 pgrep 失败: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if pid, err := strconv.Atoi(line); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// findConfigFile 查找配置文件
func (s *ApplicationService) findConfigFile(appName string) string {
	// 根据application_config_type决定配置目录
	var configDir string
	var expectedSuffix string
	
	switch s.config.Application.ApplicationConfigType {
	case "spring_cloud":
		configDir = filepath.Join(config.GetBackendDir(), "config_cloud")
		expectedSuffix = "-bootstrap" // Spring Cloud使用bootstrap配置文件
	case "spring_local":
		fallthrough
	default:
		configDir = config.GetConfigDir() // 默认使用config目录
		expectedSuffix = "" // 本地模式不需要特殊后缀
	}

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return ""
	}

	// 可能的配置文件扩展名
	extensions := []string{".yml", ".yaml", ".properties", ".ini"}

	// 尝试精确匹配
	for _, ext := range extensions {
		var configFile string
		if expectedSuffix != "" {
			// Spring Cloud模式：查找带有bootstrap后缀的文件
			configFile = filepath.Join(configDir, appName+expectedSuffix+ext)
		} else {
			// 本地模式：直接使用应用名
			configFile = filepath.Join(configDir, appName+ext)
		}
		if _, err := os.Stat(configFile); err == nil {
			return configFile
		}
	}

	// 尝试更精确的模糊匹配
	files, err := os.ReadDir(configDir)
	if err != nil {
		return ""
	}

	appNameLower := strings.ToLower(appName)
	// 移除.jar或.war后缀进行匹配
	appNameWithoutExt := strings.TrimSuffix(strings.TrimSuffix(appNameLower, ".jar"), ".war")
	
	var bestMatch string
	var bestScore int
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := strings.ToLower(file.Name())
		for _, ext := range extensions {
			if strings.HasSuffix(fileName, ext) {
				nameWithoutExt := strings.TrimSuffix(fileName, ext)
				
				// 计算匹配分数，优先选择最佳匹配
				score := s.calculateConfigMatchScore(appNameWithoutExt, nameWithoutExt, expectedSuffix)
				if score > bestScore {
					bestScore = score
					bestMatch = filepath.Join(configDir, file.Name())
				}
			}
		}
	}

	return bestMatch
}

// calculateConfigMatchScore 计算配置文件匹配分数
// 返回值越高表示匹配度越好，0表示不匹配
func (s *ApplicationService) calculateConfigMatchScore(appName, configName, expectedSuffix string) int {
	// 对于Spring Cloud模式，需要特别处理bootstrap后缀
	if expectedSuffix != "" {
		// 检查配置文件名是否包含期望的后缀
		if !strings.Contains(configName, strings.TrimPrefix(expectedSuffix, "-")) {
			// 如果不包含期望的后缀，降低分数
			// 但仍然允许部分匹配
		}
		
		// 精确匹配：应用名 + 后缀（如party-auth-bootstrap）
		if configName == appName+strings.TrimPrefix(expectedSuffix, "-") {
			return 1000
		}
		
		// 应用名前缀 + 后缀匹配（如party-bootstrap匹配party-auth.jar）
		if strings.HasPrefix(appName, configName[:len(configName)-len(strings.TrimPrefix(expectedSuffix, "-"))]) &&
		   strings.HasSuffix(configName, strings.TrimPrefix(expectedSuffix, "-")) {
			return 950
		}
	}
	
	// 精确匹配得分最高
	if appName == configName {
		return 1000
	}
	
	// 应用名以配置名开头（如party-auth匹配party-auth.yml）
	if strings.HasPrefix(appName, configName) {
		return 900
	}
	
	// 配置名以应用名开头（如party匹配party-auth.jar）
	if strings.HasPrefix(configName, appName) {
		return 800
	}
	
	// 应用名包含配置名，但要求配置名长度至少为应用名的一半
	// 这样可以避免party.yml匹配到party-auth.jar的情况
	if strings.Contains(appName, configName) && len(configName) >= len(appName)/2 {
		return 700
	}
	
	// 配置名包含应用名，但要求应用名长度至少为配置名的一半
	if strings.Contains(configName, appName) && len(appName) >= len(configName)/2 {
		return 600
	}
	
	// 不匹配
	return 0
}

// PortConnectionStats 端口连接统计信息
type PortConnectionStats struct {
	Port        int    `json:"port"`
	Connections int    `json:"connections"`
	UniqueIPs   int    `json:"unique_ips"`
	Type        string `json:"type"`
	IsListening bool   `json:"is_listening"`
	Description string `json:"description"`
}

// trySSCommand 尝试使用ss命令获取端口信息
func (s *ApplicationService) trySSCommand(port int) (*PortConnectionStats, error) {
	// 使用ss命令获取端口连接信息，ss是现代Linux系统的标准工具
	cmd := exec.Command("ss", "-tuln")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("执行ss命令失败: %w", err)
	}

	connections := 0
	portType := "TCP"
	isListening := false

	// 解析ss输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Netid") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// ss输出格式: Netid State Recv-Q Send-Q Local_Address:Port Peer_Address:Port
		protocol := strings.ToUpper(fields[0])
		state := strings.ToUpper(fields[1])
		localAddr := fields[4]

		// 确定协议类型
		if strings.Contains(protocol, "UDP") {
			portType = "UDP"
		} else if strings.Contains(protocol, "TCP") {
			portType = "TCP"
		}

		// 检查端口匹配
		portPattern := fmt.Sprintf(":%d", port)
		if strings.HasSuffix(localAddr, portPattern) {
			connections++

			// 检查是否为监听状态
			if state == "LISTEN" || state == "UNCONN" {
				isListening = true
			}
		}
	}

	// 如果找到了端口信息，获取已建立连接数和唯一IP数
	establishedConnections := 0
	uniqueIPs := make(map[string]bool)
	if isListening {
		cmd = exec.Command("ss", "-tun")
		output, err = cmd.Output()
		if err == nil {
			lines = strings.Split(string(output), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "Netid") {
					continue
				}

				estabFields := strings.Fields(line)
				if len(estabFields) >= 6 {
					state := strings.ToUpper(estabFields[1])
					localAddr := estabFields[4]
					remoteAddr := estabFields[5]

					portPattern := fmt.Sprintf(":%d", port)
					if strings.HasSuffix(localAddr, portPattern) && state == "ESTAB" {
						establishedConnections++
						
						// 提取远程IP地址进行去重
						if remoteIP := extractIPFromAddress(remoteAddr); remoteIP != "" {
							uniqueIPs[remoteIP] = true
						}
					}
				}
			}
		}
	}

	// 生成描述
	description := s.generatePortDescription(port, isListening, establishedConnections, connections, len(uniqueIPs))

	return &PortConnectionStats{
		Port:        port,
		Connections: establishedConnections,
		UniqueIPs:   len(uniqueIPs),
		Type:        portType,
		IsListening: isListening,
		Description: description,
	}, nil
}

// extractIPFromAddress 从地址字符串中提取IP地址
func extractIPFromAddress(addr string) string {
	// 处理IPv4地址格式: 192.168.1.1:8080
	if lastColon := strings.LastIndex(addr, ":"); lastColon != -1 {
		ip := addr[:lastColon]
		// 移除IPv6地址的方括号: [::1]:8080 -> ::1
		if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
			ip = ip[1 : len(ip)-1]
		}
		return ip
	}
	return addr
}

// tryNetstatCommand 尝试使用netstat命令获取端口信息（作为ss命令的回退方案）
func (s *ApplicationService) tryNetstatCommand(port int) (*PortConnectionStats, error) {
	// 使用netstat命令获取端口连接信息
	cmd := exec.Command("netstat", "-an")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("执行netstat命令失败: %w", err)
	}

	connections := 0
	portType := "TCP"
	isListening := false
	establishedConnections := 0
	uniqueIPs := make(map[string]bool)

	// 解析netstat输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// 确定协议类型
		protocol := strings.ToUpper(fields[0])
		if strings.Contains(protocol, "UDP") {
			portType = "UDP"
		} else if strings.Contains(protocol, "TCP") {
			portType = "TCP"
		}

		// 获取本地地址和远程地址
		localAddr := fields[1]
		remoteAddr := ""
		if len(fields) >= 3 {
			remoteAddr = fields[2]
		}

		// 改进的端口匹配逻辑 - 支持Linux格式
		portPattern := fmt.Sprintf(":%d", port)
		if strings.HasSuffix(localAddr, portPattern) {
			// 确保是精确匹配端口号
			beforePort := strings.TrimSuffix(localAddr, portPattern)
			// 检查端口前面是否是有效的IP地址格式
			if beforePort == "" ||
				strings.HasSuffix(beforePort, ".") ||
				strings.HasSuffix(beforePort, ":") ||
				beforePort == "0.0.0.0" ||
				beforePort == "127.0.0.1" ||
				beforePort == ":::" ||
				beforePort == "[::]" ||
				strings.Contains(beforePort, ".") {

				connections++

				// 检查状态
				if len(fields) >= 4 {
					state := strings.ToUpper(fields[3])
					switch state {
					case "LISTENING", "LISTEN":
						isListening = true
					case "ESTABLISHED":
						establishedConnections++
						// 提取远程IP地址进行去重
						if remoteAddr != "" && remoteAddr != "0.0.0.0:0" && remoteAddr != "*:*" {
							if remoteIP := extractIPFromAddress(remoteAddr); remoteIP != "" {
								uniqueIPs[remoteIP] = true
							}
						}
					}
				}
			}
		}
	}

	// 生成描述
	description := s.generatePortDescription(port, isListening, establishedConnections, connections, len(uniqueIPs))

	return &PortConnectionStats{
		Port:        port,
		Connections: establishedConnections,
		UniqueIPs:   len(uniqueIPs),
		Type:        portType,
		IsListening: isListening,
		Description: description,
	}, nil
}

// generatePortDescription 生成端口描述信息
func (s *ApplicationService) generatePortDescription(port int, isListening bool, establishedConnections int, totalConnections int, uniqueIPs int) string {
	// 设置端口类型描述
	portDescription := ""
	switch port {
	case 80:
		portDescription = "HTTP服务"
	case 443:
		portDescription = "HTTPS服务"
	case 8080:
		portDescription = "Web应用"
	case 8081:
		portDescription = "Web应用"
	case 3306:
		portDescription = "MySQL数据库"
	case 5432:
		portDescription = "PostgreSQL数据库"
	case 6379:
		portDescription = "Redis缓存"
	case 8848:
		portDescription = "Nacos注册中心"
	default:
		if port >= 8000 && port <= 9999 {
			portDescription = "应用服务"
		} else {
			portDescription = "未知服务"
		}
	}

	// 生成最终描述
	if isListening {
		// 如果端口在监听，显示服务类型和连接数
		if establishedConnections > 0 {
			if uniqueIPs > 0 {
				return fmt.Sprintf("%s (活跃连接: %d, 唯一IP: %d)", portDescription, establishedConnections, uniqueIPs)
			} else {
				return fmt.Sprintf("%s (活跃连接: %d)", portDescription, establishedConnections)
			}
		} else {
			return portDescription
		}
	} else if totalConnections > 0 {
		return "有连接但未监听"
	} else {
		return "端口关闭"
	}
}

// GetPortConnectionStats 获取指定端口的连接统计信息
func (s *ApplicationService) GetPortConnectionStats(port int) (*PortConnectionStats, error) {
	// 分层检测策略：优先使用ss命令，失败时回退到netstat命令

	// 第一层：尝试使用ss命令（现代Linux标准工具）
	if stats, err := s.trySSCommand(port); err == nil {
		return stats, nil
	}

	// 第二层：回退到netstat命令
	if stats, err := s.tryNetstatCommand(port); err == nil {
		return stats, nil
	}

	// 第三层：如果所有命令都失败，返回基础信息
	return &PortConnectionStats{
		Port:        port,
		Connections: 0,
		Type:        "TCP",
		IsListening: false,
		Description: "检测失败",
	}, nil
}
