package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ApplicationService 应用服务
type ApplicationService struct {
	config *config.Config
}

// NewApplicationService 创建应用服务实例
func NewApplicationService() *ApplicationService {
	return &ApplicationService{
		config: config.AppConfig,
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
			args = append(args, "-Dspring.config.location=file:"+app.ConfigFile)
			logger.Info("使用配置文件: %s", filepath.Base(app.ConfigFile))
		} else {
			logger.Warning("配置文件不存在，跳过配置文件参数: %s", app.ConfigFile)
		}
	} else {
		logger.Info("未找到匹配的配置文件，使用应用内部默认配置")
	}
	
	// 添加JAR文件
	args = append(args, "-jar", app.FilePath)
	
	// 创建日志目录
	if err := os.MkdirAll(filepath.Dir(app.LogFile), 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	
	// 创建日志文件
	logFile, err := os.OpenFile(app.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
	logger.Info("使用 'app-manager status' 查看应用状态")
	
	return nil
}

// StopApplication 停止应用
func (s *ApplicationService) StopApplication(app *models.Application) error {
	logger.Info("正在准备停止应用: %s", app.Name)

	// 1. 合并来自PID文件和系统进程列表的PID
	pidsToStop := make(map[int]bool)
	if app.PID > 0 {
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
		// 确保PID文件也被清理
		app.RemovePID()
		app.Status = models.StatusStopped
		return nil
	}

	// 2. 尝试优雅停止
	logger.Info("尝试优雅停止以下进程: %v", pidsToStop)
	for pid := range pidsToStop {
		process, err := os.FindProcess(pid)
		if err != nil {
			continue // 进程可能已经消失
		}
		// 发送SIGTERM信号
		process.Signal(syscall.SIGTERM)
	}

	// 3. 等待并验证
	time.Sleep(3 * time.Second) // 等待3秒让进程响应

	// 4. 强制停止仍然存在的进程
	remainingPIDs, _ := s.findPIDsByAppName(app.Name)
	if len(remainingPIDs) > 0 {
		logger.Warning("以下进程未能优雅退出，将强制终止: %v", remainingPIDs)
		for _, pid := range remainingPIDs {
			process, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			if err := process.Kill(); err != nil {
				logger.Error("强制终止PID %d 失败: %v", pid, err)
			}
		}
	}

	// 5. 最终清理
	app.RemovePID()
	app.Status = models.StatusStopped
	logger.Success("应用 %s 已成功停止。", app.Name)
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
	configDir := config.GetConfigDir()

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return ""
	}
	
	// 可能的配置文件扩展名
	extensions := []string{".yml", ".yaml", ".properties", ".ini"}
	
	// 尝试精确匹配
	for _, ext := range extensions {
		configFile := filepath.Join(configDir, appName+ext)
		if _, err := os.Stat(configFile); err == nil {
			return configFile
		}
	}
	
	// 尝试模糊匹配
	files, err := os.ReadDir(configDir)
	if err != nil {
		return ""
	}
	
	appNameLower := strings.ToLower(appName)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		fileName := strings.ToLower(file.Name())
		for _, ext := range extensions {
			if strings.HasSuffix(fileName, ext) {
				nameWithoutExt := strings.TrimSuffix(fileName, ext)
				if strings.Contains(nameWithoutExt, appNameLower) ||
					strings.Contains(appNameLower, nameWithoutExt) {
					return filepath.Join(configDir, file.Name())
				}
			}
		}
	}
	
	return ""
}