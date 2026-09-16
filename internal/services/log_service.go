package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LogService 日志服务
type LogService struct {
	config *config.Config
}

// NewLogService 创建日志服务实例
func NewLogService() *LogService {
	return &LogService{
		config: config.AppConfig,
	}
}

// ViewLog 查看应用日志
func (s *LogService) ViewLog(appName string, lines int, follow bool) error {
	// 查找应用
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}

	app := apps.FindByName(appName)
	if app == nil {
		return fmt.Errorf("未找到应用: %s", appName)
	}

	// 解析当前正在使用的日志（会话日志优先，兼容旧命名）
	logPath := ResolveCurrentLog(app.Name)
	if logPath == "" {
		logPath = app.LogFile
	}
	if logPath == "" {
		return fmt.Errorf("应用 %s 没有配置日志文件", appName)
	}

	// 检查日志文件是否存在
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("日志文件不存在: %s", logPath)
	}

	logger.Title("查看应用日志")
	logger.Info("应用名称: %s", app.Name)
	logger.Info("日志文件: %s", logPath)
	logger.Println("")

	return s.TailLogFile(logPath, lines, follow)
}

// TailLogFile 查看指定日志文件的最后若干行，follow 为 true 时实时跟踪
func (s *LogService) TailLogFile(logFile string, lines int, follow bool) error {
	if follow {
		return s.followLogWithTail(logFile, lines)
	}
	return s.tailLogWithCommand(logFile, lines)
}

// ListAppSessions 列出某个应用的全部日志会话（按启动时间倒序）
func (s *LogService) ListAppSessions(appName string) error {
	// 解析应用名称，支持模糊匹配
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}

	resolvedName := appName
	if app := apps.FindByName(appName); app != nil {
		resolvedName = app.Name
	} else if matches := apps.FindByPattern(appName); len(matches) == 1 {
		resolvedName = matches[0].Name
	}

	sessions := ListLogSessions(resolvedName)

	logger.Title("应用日志历史")
	logger.Info("应用名称: %s", resolvedName)
	logger.Println("")

	if len(sessions) == 0 {
		logger.Warning("没有找到日志文件，该应用可能尚未启动过")
		// 兼容旧命名日志
		legacy := LegacyLogPath(resolvedName)
		if _, err := os.Stat(legacy); err == nil {
			logger.Info("发现旧版命名日志: %s", legacy)
		}
		return nil
	}

	logger.Printf("共 \033[33m%d\033[0m 个运行周期日志（新的在前）:\n\n", len(sessions))
	for i, session := range sessions {
		state := "\033[32m运行中\033[0m"
		if session.Stopped {
			state = "\033[90m已停止\033[0m"
		}
		marker := " "
		if i == 0 {
			marker = "\033[33m*\033[0m"
		}

		logger.Printf("%s %2d. %s  [%s]  %s\n",
			marker, i+1,
			session.StartTime.Format("2006-01-02 15:04:05"),
			state,
			s.formatFileSize(session.Size))
		logger.Printf("      %s\n", session.Path)
	}

	logger.Println("")
	logger.Info("查看指定会话: %s logs %s --tail <序号> [-n 行数]", config.GetProgramName(), resolvedName)

	return nil
}

// TailLogSession 查看某个应用指定序号的日志会话（序号从 1 开始，1 为最新一次运行）
func (s *LogService) TailLogSession(appName string, index int, lines int, follow bool) error {
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}

	resolvedName := appName
	if app := apps.FindByName(appName); app != nil {
		resolvedName = app.Name
	} else if matches := apps.FindByPattern(appName); len(matches) == 1 {
		resolvedName = matches[0].Name
	}

	sessions := ListLogSessions(resolvedName)
	if len(sessions) == 0 {
		return fmt.Errorf("应用 %s 没有历史日志", resolvedName)
	}
	if index < 1 || index > len(sessions) {
		return fmt.Errorf("会话序号 %d 超出范围 (1-%d)", index, len(sessions))
	}

	session := sessions[index-1]

	logger.Title("查看历史日志")
	logger.Info("应用名称: %s", resolvedName)
	logger.Info("启动时间: %s (%s)", session.StartTime.Format("2006-01-02 15:04:05"), statusText(session.Stopped))
	logger.Info("日志文件: %s", session.Path)
	logger.Println("")

	return s.TailLogFile(session.Path, lines, follow)
}

// statusText 会话状态文本
func statusText(stopped bool) string {
	if stopped {
		return "已归档"
	}
	return "运行中/未归档"
}

// tailLogWithCommand 使用Linux原生tail命令显示日志文件的最后几行
func (s *LogService) tailLogWithCommand(logFile string, lines int) error {
	// 使用tail命令显示最后几行
	cmd := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logFile)
	
	// 获取命令输出
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("执行tail命令失败: %w", err)
	}

	// 按行处理输出并着色
	lines_output := strings.Split(string(output), "\n")
	for _, line := range lines_output {
		if strings.TrimSpace(line) != "" {
			s.printLogLine(line)
		}
	}

	return nil
}

// CreateLogrotateConfig 创建统一的logrotate配置文件
func (ls *LogService) CreateLogrotateConfig(configName, description string) error {
	if configName == "" {
		return fmt.Errorf("配置名称不能为空")
	}

	// 直接从全局配置获取项目根目录
	projectRoot := config.GetProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("项目根目录未在配置中设置")
	}

	// 生成logrotate配置内容
	configContent := ls.generateLogrotateConfig(projectRoot, description)

	// 配置文件路径
	configPath := fmt.Sprintf("/etc/logrotate.d/%s", configName)

	// 写入配置文件
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("写入logrotate配置文件失败: %v", err)
	}

	logger.Info("已创建logrotate配置: %s", configPath)
	ls.printInstructions(configName)
	return nil
}

// generateLogrotateConfig 生成logrotate配置内容
func (ls *LogService) generateLogrotateConfig(projectRoot, description string) string {
	var configBuilder strings.Builder
	logsDir := config.GetLogsDir()
	rotateLogsDir := config.GetRotateLogsDir()

	if description != "" {
		configBuilder.WriteString(fmt.Sprintf("# %s\n", description))
	}
	configBuilder.WriteString("# App Manager 统一日志轮转配置\n")
	configBuilder.WriteString(fmt.Sprintf("# 项目路径: %s\n\n", projectRoot))

	// 会话日志: <app>-yyyy-MM-dd-HHmmss.log / <app>-yyyy-MM-dd-HHmmss-停止.log
	// 注意: 当前正在写入的日志由 app-manager 在 stop 时归档，logrotate 只兜底处理超大/超期的历史文件
	configBuilder.WriteString("# 会话日志（按运行周期留痕，由 app-manager 自动归档）\n")
	configBuilder.WriteString(fmt.Sprintf("%s/*-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]*.log {\n", logsDir))
	configBuilder.WriteString(fmt.Sprintf("    %s\n", ls.config.Logrotate.LogrotateCycle))
	configBuilder.WriteString("    missingok\n")
	configBuilder.WriteString(fmt.Sprintf("    rotate %d\n", ls.config.Logrotate.LogrotateRotate))
	configBuilder.WriteString(fmt.Sprintf("    size %s\n", ls.config.Logrotate.LogrotateSize))
	configBuilder.WriteString("    compress\n")
	configBuilder.WriteString("    delaycompress\n")
	configBuilder.WriteString("    notifempty\n")
	configBuilder.WriteString("    copytruncate\n")
	configBuilder.WriteString("    create 644 root root\n")
	configBuilder.WriteString(fmt.Sprintf("    olddir %s\n", rotateLogsDir))
	configBuilder.WriteString("}\n\n")

	// 兼容旧版固定命名的日志文件 <app>.log
	configBuilder.WriteString("# 旧版固定命名日志（兼容保留）\n")
	configBuilder.WriteString(fmt.Sprintf("%s/*.log {\n", logsDir))
	configBuilder.WriteString(fmt.Sprintf("    %s\n", ls.config.Logrotate.LogrotateCycle))
	configBuilder.WriteString("    missingok\n")
	configBuilder.WriteString(fmt.Sprintf("    rotate %d\n", ls.config.Logrotate.LogrotateRotate))
	configBuilder.WriteString(fmt.Sprintf("    size %s\n", ls.config.Logrotate.LogrotateSize))
	configBuilder.WriteString("    compress\n")
	configBuilder.WriteString("    delaycompress\n")
	configBuilder.WriteString("    notifempty\n")
	configBuilder.WriteString("    copytruncate\n")
	configBuilder.WriteString("    create 644 root root\n")
	configBuilder.WriteString(fmt.Sprintf("    olddir %s\n", rotateLogsDir))
	configBuilder.WriteString("}\n")

	return configBuilder.String()
}

// printInstructions 打印logrotate配置说明
func (ls *LogService) printInstructions(configName string) {
	logger.Println("")
	logger.Success("Logrotate配置创建成功！")
	logger.Println("")

	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Printf("\033[33m\033[1m  下一步操作说明\033[0m\n")
	logger.Println("═══════════════════════════════════════════════════════════")
	logger.Println("")
	logger.Info("配置文件已创建在: /etc/logrotate.d/%s", configName)
	logger.Println("")
	logger.Warning("请检查配置文件内容是否符合预期，特别是以下几项：")
	logger.Printf("  - \033[37m轮转周期 (cycle):\033[0m %s\n", ls.config.Logrotate.LogrotateCycle)
	logger.Printf("  - \033[37m保留文件数 (rotate):\033[0m %d\n", ls.config.Logrotate.LogrotateRotate)
	logger.Printf("  - \033[37m文件大小 (size):\033[0m %s\n", ls.config.Logrotate.LogrotateSize)
	logger.Println("")
	logger.Info("你可以通过以下命令手动测试logrotate配置：")
	logger.Printf("  \033[37m# 强制执行所有logrotate任务\033[0m\n")
	logger.Printf("  logrotate -f /etc/logrotate.conf\n")
	logger.Printf("  \033[37m# 强制执行并显示详细输出\033[0m\n")
	logger.Printf("  logrotate -vf /etc/logrotate.conf\n")
	logger.Printf("  \033[37m# 调试模式，模拟执行，不实际操作\033[0m\n")
	logger.Printf("  logrotate -d /etc/logrotate.conf\n")
	logger.Println("")
	logger.Warning("注意：logrotate通常由cron每日自动执行，无需手动干预。")
}

// getProjectRoot 获取项目根目录 (该函数将不再被使用)
func (ls *LogService) getProjectRoot() (string, error) {
	// 从当前工作目录开始查找项目根目录
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// 查找包含app-manager配置文件的目录
	for {
		configPath := filepath.Join(currentDir, config.GetConfigFileName())
		if _, err := os.Stat(configPath); err == nil {
			return currentDir, nil
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	return "", fmt.Errorf("未找到项目根目录")
}

// followLogWithTail 使用Linux原生tail -f命令实时跟踪日志文件
func (s *LogService) followLogWithTail(logFile string, lines int) error {
	logger.Info("开始实时跟踪日志文件 (按 Ctrl+C 退出)...")
	logger.Println("")

	// 使用tail -f命令实时跟踪日志
	cmd := exec.Command("tail", "-f", "-n", fmt.Sprintf("%d", lines), logFile)
	
	// 获取命令的stdout管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建stdout管道失败: %w", err)
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动tail命令失败: %w", err)
	}

	// 创建上下文和信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在goroutine中监听信号
	go func() {
		select {
		case <-sigChan:
			logger.Info("\n收到中断信号，正在退出日志跟踪模式...")
			// 终止tail命令
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			cancel()
		case <-ctx.Done():
		}
	}()

	// 读取tail命令的输出
	scanner := bufio.NewScanner(stdout)
	go func() {
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				s.printLogLine(scanner.Text())
			}
		}
	}()

	// 等待信号或命令结束
	select {
	case <-ctx.Done():
		// 清理信号处理
		signal.Stop(sigChan)
		close(sigChan)
		logger.Success("日志跟踪已安全退出")
		return nil
	}
}

// printLogLine 打印日志行（带颜色）
func (s *LogService) printLogLine(line string) {
	// 根据日志级别添加颜色
	if strings.Contains(line, "ERROR") || strings.Contains(line, "FATAL") {
		fmt.Printf("\033[31m%s\033[0m\n", line) // 红色
	} else if strings.Contains(line, "WARN") {
		fmt.Printf("\033[33m%s\033[0m\n", line) // 黄色
	} else if strings.Contains(line, "INFO") {
		fmt.Printf("\033[32m%s\033[0m\n", line) // 绿色
	} else if strings.Contains(line, "DEBUG") {
		fmt.Printf("\033[36m%s\033[0m\n", line) // 青色
	} else {
		fmt.Println(line) // 默认颜色
	}
}

// RotateLogs 执行日志轮转
func (s *LogService) RotateLogs() error {
	logger.Title("日志轮转")
	logger.Info("开始执行日志轮转...")

	logsDir := config.GetLogsDir()

	// 检查日志目录是否存在
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		logger.Warning("日志目录不存在: %s", logsDir)
		return nil
	}

	// 获取所有日志文件
	logFiles, err := s.findLogFiles(logsDir)
	if err != nil {
		return fmt.Errorf("查找日志文件失败: %w", err)
	}

	if len(logFiles) == 0 {
		logger.Info("没有找到需要轮转的日志文件")
		return nil
	}

	logger.Info("找到 %d 个日志文件", len(logFiles))

	rotatedCount := 0
	for _, logFile := range logFiles {
		if s.shouldRotateLog(logFile) {
			if err := s.rotateLogFile(logFile); err != nil {
				logger.Warning("轮转日志文件失败 %s: %v", logFile, err)
			} else {
				rotatedCount++
			}
		}
	}

	logger.Success("日志轮转完成，共轮转 %d 个文件", rotatedCount)

	// 清理旧的轮转日志
	rotateLogsDir := config.GetRotateLogsDir()
	s.cleanOldRotatedLogs(rotateLogsDir)

	return nil
}

// findLogFiles 查找日志文件
func (s *LogService) findLogFiles(logsDir string) ([]string, error) {
	var logFiles []string

	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 只处理 .log 和 .out 文件
		if strings.HasSuffix(info.Name(), ".log") || strings.HasSuffix(info.Name(), ".out") {
			// 跳过已经轮转的文件
			if !strings.Contains(info.Name(), ".") ||
				strings.HasSuffix(info.Name(), ".log") ||
				strings.HasSuffix(info.Name(), ".out") {
				logFiles = append(logFiles, path)
			}
		}

		return nil
	})

	return logFiles, err
}

// shouldRotateLog 判断是否需要轮转日志
func (s *LogService) shouldRotateLog(logFile string) bool {
	stat, err := os.Stat(logFile)
	if err != nil {
		return false
	}

	// 检查文件大小
	maxSize := s.parseSize(s.config.Logrotate.LogrotateSize)
	if stat.Size() >= maxSize {
		return true
	}

	// 检查时间周期
	if s.config.Logrotate.LogrotateCycle == "daily" {
		// 检查文件是否超过一天
		return time.Since(stat.ModTime()) > 24*time.Hour
	} else if s.config.Logrotate.LogrotateCycle == "weekly" {
		// 检查文件是否超过一周
		return time.Since(stat.ModTime()) > 7*24*time.Hour
	}

	return false
}

// parseSize 解析大小字符串
func (s *LogService) parseSize(sizeStr string) int64 {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	// 提取数字和单位
	re := regexp.MustCompile(`^(\d+)([KMGT]?)B?$`)
	matches := re.FindStringSubmatch(sizeStr)
	if len(matches) != 3 {
		return 10 * 1024 * 1024 // 默认10MB
	}

	size, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 10 * 1024 * 1024 // 默认10MB
	}

	unit := matches[2]
	switch unit {
	case "K":
		return size * 1024
	case "M":
		return size * 1024 * 1024
	case "G":
		return size * 1024 * 1024 * 1024
	case "T":
		return size * 1024 * 1024 * 1024 * 1024
	default:
		return size
	}
}

// rotateLogFile 轮转单个日志文件
func (s *LogService) rotateLogFile(logFile string) error {
	logger.Info("轮转日志文件: %s", logFile)

	// 生成轮转后的文件名
	timestamp := time.Now().Format("20060102_150405")
	rotatedFile := fmt.Sprintf("%s.%s", logFile, timestamp)

	// 重命名原文件
	if err := os.Rename(logFile, rotatedFile); err != nil {
		return fmt.Errorf("重命名日志文件失败: %w", err)
	}

	// 创建新的空日志文件
	newFile, err := os.Create(logFile)
	if err != nil {
		// 如果创建失败，尝试恢复原文件
		os.Rename(rotatedFile, logFile)
		return fmt.Errorf("创建新日志文件失败: %w", err)
	}
	newFile.Close()

	// 设置文件权限
	if err := os.Chmod(logFile, 0644); err != nil {
		logger.Warning("设置日志文件权限失败: %v", err)
	}

	logger.Info("日志文件轮转完成: %s -> %s", logFile, rotatedFile)
	return nil
}

// cleanOldRotatedLogs 清理旧的轮转日志
func (s *LogService) cleanOldRotatedLogs(logsDir string) {
	if s.config.Logrotate.LogrotateRotate <= 0 {
		return
	}

	logger.Info("清理旧的轮转日志文件...")

	// 按应用分组查找轮转日志
	appLogs := make(map[string][]string)

	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		name := info.Name()
		// 匹配轮转日志文件格式: app.log.20240101_120000
		if strings.Contains(name, ".log.") || strings.Contains(name, ".out.") {
			// 提取应用名
			var appName string
			if strings.Contains(name, ".log.") {
				appName = strings.Split(name, ".log.")[0] + ".log"
			} else {
				appName = strings.Split(name, ".out.")[0] + ".out"
			}

			appLogs[appName] = append(appLogs[appName], path)
		}

		return nil
	})

	// 为每个应用清理旧日志
	for appName, logs := range appLogs {
		s.cleanAppRotatedLogs(appName, logs)
	}
}

// cleanAppRotatedLogs 清理单个应用的旧轮转日志
func (s *LogService) cleanAppRotatedLogs(appName string, logs []string) {
	if len(logs) <= s.config.Logrotate.LogrotateRotate {
		return
	}

	// 按修改时间排序（最新的在前）
	for i := 0; i < len(logs)-1; i++ {
		for j := i + 1; j < len(logs); j++ {
			stat1, _ := os.Stat(logs[i])
			stat2, _ := os.Stat(logs[j])
			if stat1.ModTime().Before(stat2.ModTime()) {
				logs[i], logs[j] = logs[j], logs[i]
			}
		}
	}

	// 删除多余的日志文件
	for i := s.config.Logrotate.LogrotateRotate; i < len(logs); i++ {
		if err := os.Remove(logs[i]); err != nil {
			logger.Warning("删除旧轮转日志失败 %s: %v", logs[i], err)
		} else {
			logger.Info("删除旧轮转日志: %s", filepath.Base(logs[i]))
		}
	}
}

// ListLogs 列出所有应用的日志文件
func (s *LogService) ListLogs() error {
	logger.Title("应用日志文件列表")

	// 获取所有应用
	appService := NewApplicationService()
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}

	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}

	for _, app := range apps {
		logger.Printf("应用: \033[33m%s\033[0m\n", app.Name)

		logPath := ResolveCurrentLog(app.Name)
		if logPath == "" {
			logPath = app.LogFile
		}

		if logPath == "" {
			logger.Printf("  日志文件: \033[31m未配置\033[0m\n")
		} else if _, err := os.Stat(logPath); os.IsNotExist(err) {
			logger.Printf("  日志文件: \033[31m%s (不存在)\033[0m\n", logPath)
		} else {
			stat, _ := os.Stat(logPath)
			size := s.formatFileSize(stat.Size())
			modTime := stat.ModTime().Format("2006-01-02 15:04:05")
			logger.Printf("  当前日志: \033[32m%s\033[0m\n", logPath)
			logger.Printf("  文件大小: %s\n", size)
			logger.Printf("  修改时间: %s\n", modTime)
		}

		if sessions := CountLogSessions(app.Name); sessions > 0 {
			logger.Printf("  历史会话: %d 个 (查看: %s logs %s --history)\n", sessions, config.GetProgramName(), app.Name)
		}
		logger.Println("")
	}

	return nil
}

// formatFileSize 格式化文件大小
func (s *LogService) formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
	} else {
		return fmt.Sprintf("%.2f GB", float64(size)/(1024*1024*1024))
	}
}
