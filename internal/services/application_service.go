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
	"runtime"
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
	
	// 获取所有运行中的Java进程
	runningJavaProcesses := s.getRunningJavaProcesses()
	
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
			
			// 检查实际运行的Java进程
			s.matchRunningProcess(app, runningJavaProcesses)
			
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

// JavaProcess 表示一个Java进程
type JavaProcess struct {
	PID     int
	Command string
	JarPath string
}

// getRunningJavaProcesses 获取所有运行中的Java进程
func (s *ApplicationService) getRunningJavaProcesses() []JavaProcess {
	var processes []JavaProcess
	
	if runtime.GOOS == "windows" {
		// Windows系统使用PowerShell命令获取更准确的结果
		logger.Info("🔍 正在检测Windows系统上的Java进程...")
		
		// 使用PowerShell的Get-WmiObject命令
		cmd := exec.Command("powershell", "-Command", 
			"Get-WmiObject -Class Win32_Process -Filter \"name='java.exe'\" | Select-Object ProcessId, CommandLine | ConvertTo-Csv -NoTypeInformation")
		output, err := cmd.Output()
		if err != nil {
			logger.Warning("获取Windows进程列表失败: %v", err)
			// 回退到wmic命令
			logger.Info("🔄 回退使用wmic命令...")
			cmd = exec.Command("wmic", "process", "where", "name='java.exe'", "get", "ProcessId,CommandLine", "/format:csv")
			output, err = cmd.Output()
			if err != nil {
				logger.Warning("wmic命令也失败: %v", err)
				return processes
			}
		}
		
		logger.Info("📋 命令输出长度: %d 字符", len(output))
		outputStr := string(output)
		logger.Info("📋 原始输出: %s", outputStr)
		
		lines := strings.Split(outputStr, "\n")
		logger.Info("📋 解析到 %d 行输出", len(lines))
		
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.Contains(line, "ProcessId") || strings.Contains(line, "CommandLine") || strings.Contains(line, "Node") {
				continue
			}
			
			logger.Info("📋 处理第 %d 行: %s", i+1, line)
			
			// 处理CSV格式的输出
			if strings.Contains(line, ",") {
				// 使用正则表达式解析CSV，处理包含逗号的命令行
				parts := strings.SplitN(line, ",", 2)
				if len(parts) >= 2 {
					pidStr := strings.Trim(parts[0], "\"")
					commandLine := strings.Trim(parts[1], "\"")
					
					// 如果是wmic格式，可能顺序不同
					if !strings.Contains(commandLine, "java") && strings.Contains(pidStr, "java") {
						// 交换顺序
						pidStr, commandLine = commandLine, pidStr
					}
					
					logger.Info("📋 PID字符串: %s", pidStr)
					logger.Info("📋 命令行: %s", commandLine)
					
					// 检查是否包含.jar
					if strings.Contains(commandLine, ".jar") {
						logger.Info("✅ 发现包含.jar的Java进程")
						if pid, err := strconv.Atoi(strings.TrimSpace(pidStr)); err == nil {
							jarPath := s.extractJarPath(commandLine)
							logger.Info("📋 提取的JAR路径: %s", jarPath)
							if jarPath != "" {
								processes = append(processes, JavaProcess{
									PID:     pid,
									Command: commandLine,
									JarPath: jarPath,
								})
								logger.Info("✅ 添加Java进程: PID=%d, JAR=%s", pid, jarPath)
							}
						} else {
							logger.Warning("❌ 解析PID失败: %v", err)
						}
					}
				}
			}
		}
		
		logger.Info("🎯 总共发现 %d 个Java进程", len(processes))
	} else {
		// Linux/Unix系统使用ps命令
		cmd := exec.Command("ps", "-eo", "pid,cmd")
		output, err := cmd.Output()
		if err != nil {
			logger.Warning("获取进程列表失败: %v", err)
			return processes
		}
		
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "java") && strings.Contains(line, ".jar") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if pid, err := strconv.Atoi(parts[0]); err == nil {
						command := strings.Join(parts[1:], " ")
						jarPath := s.extractJarPath(command)
						if jarPath != "" {
							processes = append(processes, JavaProcess{
								PID:     pid,
								Command: command,
								JarPath: jarPath,
							})
						}
					}
				}
			}
		}
	}
	
	return processes
}

// extractJarPath 从命令行中提取JAR文件路径
func (s *ApplicationService) extractJarPath(command string) string {
	// 查找 -jar 参数后的JAR文件路径
	parts := strings.Fields(command)
	for i, part := range parts {
		if part == "-jar" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// matchRunningProcess 匹配运行中的进程
func (s *ApplicationService) matchRunningProcess(app *models.Application, processes []JavaProcess) {
	for _, process := range processes {
		// 检查JAR文件路径是否匹配
		if strings.HasSuffix(process.JarPath, app.Name+".jar") {
			// 如果PID文件中的PID与实际进程不匹配，更新PID
			if app.PID != process.PID {
				logger.Info("发现应用 %s 的实际PID %d，更新PID文件", app.Name, process.PID)
				app.PID = process.PID
				
				// 如果 StartTime 为零值，设置为当前时间（估算启动时间）
				if app.StartTime.IsZero() {
					app.StartTime = time.Now().Add(-5 * time.Minute) // 假设已运行5分钟
					logger.Info("设置应用 %s 的估算启动时间", app.Name)
				}
				
				// 保存正确的PID到文件
				pidFile := filepath.Join(config.GetPidsDir(), app.Name+".pid")
				if err := app.SavePID(pidFile); err != nil {
					logger.Warning("保存PID文件失败 %s: %v", app.Name, err)
				}
			}
			break
		}
	}
}

// StartApplication 启动应用
func (s *ApplicationService) StartApplication(app *models.Application) error {
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
	// 设置进程属性（根据操作系统）
	s.setSysProcAttr(cmd)
	
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
	app.StartTime = time.Now()
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
	if !app.IsRunning() {
		logger.Warning("应用 %s 未运行", app.Name)
		return nil
	}
	
	logger.Info("正在停止应用: %s (PID: %d)", app.Name, app.PID)
	
	process, err := os.FindProcess(app.PID)
	if err != nil {
		return fmt.Errorf("查找进程失败: %w", err)
	}
	
	app.Status = models.StatusStopping
	
	// 优雅停止 - 发送SIGTERM信号
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("发送停止信号失败: %w", err)
	}
	
	logger.Info("发送停止信号，等待应用优雅退出...")
	
	// 等待进程退出，最多等待10秒
	for i := 0; i < 10; i++ {
		if !app.IsRunning() {
			break
		}
		time.Sleep(1 * time.Second)
		
		if i%3 == 0 {
			logger.Info("等待中... (%d/10秒)", i+1)
		}
	}
	
	// 检查是否已停止
	if app.IsRunning() {
		logger.Warning("应用未能优雅停止，强制终止...")
		if err := process.Kill(); err != nil {
			return fmt.Errorf("强制终止应用失败: %w", err)
		}
		logger.Success("应用强制停止成功: %s", app.Name)
	} else {
		logger.Success("应用停止成功: %s", app.Name)
	}
	
	// 清理PID文件
	app.RemovePID()
	app.Status = models.StatusStopped
	
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
	
	status := &models.ApplicationStatus{
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
		Port:        app.GetPortString(),
	}
	
	return status
}

// updateApplicationRealStatus 更新应用的真实状态
func (s *ApplicationService) updateApplicationRealStatus(app *models.Application) {
	// 首先检查是否有匹配的Java进程在运行
	runningProcesses := s.getRunningJavaProcesses()
	var matchingPID int
	
	// 查找匹配的进程
	for _, process := range runningProcesses {
		if strings.Contains(process.JarPath, app.Name+".jar") {
			matchingPID = process.PID
			break
		}
	}
	
	// 如果找到匹配的进程，更新PID
	if matchingPID > 0 {
		if app.PID != matchingPID {
			logger.Info("发现应用 %s 的实际PID %d，更新PID文件", app.Name, matchingPID)
			app.PID = matchingPID
			
			// 如果 StartTime 为零值，设置为当前时间（估算启动时间）
			if app.StartTime.IsZero() {
				app.StartTime = time.Now().Add(-5 * time.Minute) // 假设已运行5分钟
				logger.Info("设置应用 %s 的估算启动时间", app.Name)
			}
			
			pidFile := filepath.Join(config.GetPidsDir(), app.Name+".pid")
			if err := app.SavePID(pidFile); err != nil {
				logger.Warning("保存PID文件失败 %s: %v", app.Name, err)
			}
		}
		
		// 检查应用是否真正启动成功
		if s.checkApplicationStartupStatus(app) {
			app.Status = models.StatusRunning
		} else {
			// 进程在运行但可能还在启动中
			app.Status = models.StatusStarting
		}
	} else {
		// 没有找到匹配的进程
		if app.PID > 0 {
			// 如果应用有PID，检查进程是否还在运行
			if !app.IsRunning() {
				// 进程已经不存在，更新状态
				app.Status = models.StatusStopped
				app.RemovePID()
			}
		} else {
			// 没有PID，状态应该是停止
			app.Status = models.StatusStopped
		}
	}
}

// checkApplicationStartupStatus 检查应用启动状态
func (s *ApplicationService) checkApplicationStartupStatus(app *models.Application) bool {
	// 如果没有日志文件，只能基于进程存在性判断
	if app.LogFile == "" {
		return app.IsRunning()
	}
	
	// 检查日志文件是否存在
	if _, err := os.Stat(app.LogFile); os.IsNotExist(err) {
		return false
	}
	
	// 检查日志中的启动成功标志
	if s.checkStartupSuccess(app.LogFile) {
		return true
	}
	
	// 检查是否有启动错误
	if s.checkStartupError(app.LogFile) {
		return false
	}
	
	// 如果应用运行时间超过2分钟且进程还在，认为启动成功
	if app.IsRunning() && time.Since(app.StartTime) > 2*time.Minute {
		return true
	}
	
	// 其他情况认为还在启动中
	return false
}





// waitForStartup 等待应用启动
func (s *ApplicationService) waitForStartup(app *models.Application) error {
	logger.Info("启动应用进程...")
	
	// 简单等待3秒，确保进程启动
	time.Sleep(3 * time.Second)
	
	// 检查进程是否还存在
	if !app.IsRunning() {
		app.RemovePID()
		return fmt.Errorf("应用进程启动失败或立即退出")
	}
	
	// 设置状态为运行中
	app.Status = models.StatusRunning
	logger.Success("应用进程启动成功: %s (PID: %d)", app.Name, app.PID)
	logger.Info("应用状态请通过 'status' 命令查看，启动完成需要一定时间")
	
	return nil
}

// checkStartupSuccess 检查启动成功标志
func (s *ApplicationService) checkStartupSuccess(logFile string) bool {
	file, err := os.Open(logFile)
	if err != nil {
		return false
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		// Spring Boot 启动成功标志
		if strings.Contains(line, "started") && strings.Contains(line, "seconds") ||
			strings.Contains(line, "application") && strings.Contains(line, "started") ||
			strings.Contains(line, "tomcat started on port") ||
			strings.Contains(line, "jetty started on port") ||
			strings.Contains(line, "undertow started on port") ||
			strings.Contains(line, "netty started on port") ||
			// 更通用的启动成功标志
			strings.Contains(line, "startup completed") ||
			strings.Contains(line, "server started") ||
			strings.Contains(line, "service started") ||
			strings.Contains(line, "initialization completed") ||
			strings.Contains(line, "ready to accept connections") ||
			strings.Contains(line, "listening on port") ||
			// Spring 相关
			strings.Contains(line, "spring") && strings.Contains(line, "started") ||
			strings.Contains(line, "context initialization completed") ||
			// 其他常见标志
			strings.Contains(line, "boot completed") ||
			strings.Contains(line, "startup successful") {
			return true
		}
	}
	
	return false
}

// checkStartupError 检查启动错误
func (s *ApplicationService) checkStartupError(logFile string) bool {
	file, err := os.Open(logFile)
	if err != nil {
		return false
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		if strings.Contains(line, "exception") ||
			strings.Contains(line, "error") && strings.Contains(line, "failed") ||
			strings.Contains(line, "cannot") && strings.Contains(line, "start") {
			return true
		}
	}
	
	return false
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

// isWindows 检查是否为Windows系统
func isWindows() bool {
	return os.PathSeparator == '\\'
}

// ParseBatchNumbers 解析批量操作的序号（支持逗号分隔和范围表达式）
// 例如: "1,2,3" 或 "1-5" 或 "1,3-5,7"
func ParseBatchNumbers(input string) ([]int, error) {
	var numbers []int
	
	// 按逗号分割
	parts := strings.Split(input, ",")
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		// 检查是否是范围表达式（包含连字符）
		if strings.Contains(part, "-") {
			rangeNumbers, err := parseRangeExpression(part)
			if err != nil {
				return nil, fmt.Errorf("解析范围表达式 '%s' 失败: %v", part, err)
			}
			numbers = append(numbers, rangeNumbers...)
		} else {
			// 单个数字
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("无效的数字: %s", part)
			}
			if num <= 0 {
				return nil, fmt.Errorf("序号必须大于0: %d", num)
			}
			numbers = append(numbers, num)
		}
	}
	
	// 去重并排序
	uniqueNumbers := make(map[int]bool)
	for _, num := range numbers {
		uniqueNumbers[num] = true
	}
	
	var result []int
	for num := range uniqueNumbers {
		result = append(result, num)
	}
	
	// 简单排序
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i] > result[j] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	
	return result, nil
}

// parseRangeExpression 解析范围表达式，如 "1-5"
func parseRangeExpression(expr string) ([]int, error) {
	parts := strings.Split(expr, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("无效的范围表达式格式")
	}
	
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("无效的起始数字: %s", parts[0])
	}
	
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("无效的结束数字: %s", parts[1])
	}
	
	if start <= 0 || end <= 0 {
		return nil, fmt.Errorf("序号必须大于0")
	}
	
	if start > end {
		return nil, fmt.Errorf("起始序号不能大于结束序号")
	}
	
	var numbers []int
	for i := start; i <= end; i++ {
		numbers = append(numbers, i)
	}
	
	return numbers, nil
}

// ExecuteBatchOperation 执行批量操作
func (s *ApplicationService) ExecuteBatchOperation(operation string, numbers []int, apps models.ApplicationList) error {
	if len(numbers) == 0 {
		return fmt.Errorf("没有指定要操作的应用序号")
	}
	
	var targetApps []*models.Application
	var invalidNumbers []int
	
	// 验证序号并收集目标应用
	for _, num := range numbers {
		if num < 1 || num > len(apps) {
			invalidNumbers = append(invalidNumbers, num)
			continue
		}
		targetApps = append(targetApps, apps[num-1])
	}
	
	// 如果有无效序号，报告错误
	if len(invalidNumbers) > 0 {
		var invalidStrs []string
		for _, num := range invalidNumbers {
			invalidStrs = append(invalidStrs, strconv.Itoa(num))
		}
		return fmt.Errorf("无效的应用序号: %s (有效范围: 1-%d)", strings.Join(invalidStrs, ", "), len(apps))
	}
	
	if len(targetApps) == 0 {
		return fmt.Errorf("没有找到要操作的应用")
	}
	
	// 显示将要操作的应用
	logger.Info("将要%s以下应用:", getOperationName(operation))
	for i, app := range targetApps {
		logger.Printf("  %d. %s", i+1, app.Name)
	}
	logger.Println("")
	
	// 执行操作
	var successCount, failCount int
	for _, app := range targetApps {
		logger.Info("正在%s应用: %s", getOperationName(operation), app.Name)
		
		var err error
		switch operation {
		case "s", "start":
			err = s.StartApplication(app)
		case "r", "restart":
			err = s.RestartApplication(app)
		case "k", "stop":
			err = s.StopApplication(app)
		default:
			return fmt.Errorf("不支持的操作: %s", operation)
		}
		
		if err != nil {
			logger.Error("%s应用 %s 失败: %v", getOperationName(operation), app.Name, err)
			failCount++
		} else {
			logger.Success("%s应用 %s 成功", getOperationName(operation), app.Name)
			successCount++
		}
		logger.Println("")
	}
	
	// 显示操作结果统计
	logger.Info("批量操作完成:")
	logger.Printf("  成功: %d 个应用", successCount)
	if failCount > 0 {
		logger.Printf("  失败: %d 个应用", failCount)
	}
	
	return nil
}

// getOperationName 获取操作名称
func getOperationName(operation string) string {
	switch operation {
	case "s", "start":
		return "启动"
	case "r", "restart":
		return "重启"
	case "k", "stop":
		return "停止"
	default:
		return "操作"
	}
}