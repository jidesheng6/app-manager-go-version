package cmd

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"app-manager-go-v1/internal/services"
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	statusDetail bool
	statusJSON   bool
)

// statusCmd 状态命令
var statusCmd = &cobra.Command{
	Use:   "status [application_name]",
	Short: "查看应用程序状态",
	Long: `查看指定应用程序或所有应用程序的运行状态。

如果不指定应用名称，将显示所有应用的状态概览。
如果指定应用名称，将显示该应用的详细状态信息。

状态信息包括：
- 应用名称和文件路径
- 运行状态（运行中/已停止/启动中/停止中）
- 进程ID（如果正在运行）
- 运行时长
- 内存使用情况
- CPU使用率
- 日志文件路径

使用示例：
  app-manager status                  # 查看所有应用状态
  app-manager status myapp            # 查看指定应用状态
  app-manager status --detail         # 查看详细状态信息
  app-manager status --json           # 以JSON格式输出`,
	Args: cobra.MaximumNArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		appService := services.NewApplicationService()
		
		if len(args) > 0 {
			// 查看指定应用状态
			appName := args[0]
			if err := showSingleApplicationStatus(appService, appName); err != nil {
				handleError(err, 1)
			}
		} else {
			// 查看所有应用状态并进入交互模式
			if err := showAllApplicationsStatusWithInteractive(appService); err != nil {
				handleError(err, 1)
			}
		}
	},
}

func init() {
	statusCmd.Flags().BoolVarP(&statusDetail, "detail", "d", false, "显示详细状态信息")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "以JSON格式输出")
}

// showSingleApplicationStatus 显示单个应用状态
func showSingleApplicationStatus(appService *services.ApplicationService, appName string) error {
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	// 查找应用
	app := apps.FindByName(appName)
	if app == nil {
		// 尝试模糊匹配
		matchedApps := apps.FindByPattern(appName)
		if len(matchedApps) == 0 {
			return fmt.Errorf("未找到应用: %s", appName)
		} else if len(matchedApps) > 1 {
			logger.Error("找到多个匹配的应用:")
			for _, app := range matchedApps {
				logger.Printf("  - %s", app.Name)
			}
			return fmt.Errorf("请指定更精确的应用名称")
		}
		app = matchedApps[0]
	}
	
	// 获取应用状态
	status := appService.GetApplicationStatus(app)
	
	if statusJSON {
		// TODO: 实现JSON输出
		logger.Info("JSON输出功能待实现")
		return nil
	}
	
	// 显示详细状态
	logger.Title("应用状态详情")
	showDetailedStatus(status)
	
	return nil
}

// executeOperationOnAllAppsOptimized 对所有应用执行操作（优化版本）
func executeOperationOnAllAppsOptimized(appService *services.ApplicationService, apps models.ApplicationList, operation string) error {
	// 在执行操作前，强制更新所有应用的状态
	logger.Info("正在更新应用状态...")
	for _, app := range apps {
		app.UpdateStatus()
	}
	
	switch operation {
	case "s":
		logger.Info("启动所有已停止的应用")
		stoppedApps := apps.Stopped()
		if len(stoppedApps) == 0 {
			logger.Info("没有已停止的应用")
			return nil
		}
		
		successCount := 0
		for _, app := range stoppedApps {
			logger.Info("启动应用: %s", app.Name)
			if err := appService.StartApplication(app); err != nil {
				logger.Error("启动应用失败 %s: %v", app.Name, err)
			} else {
				successCount++
			}
		}
		logger.Info("启动完成: 成功 %d 个，失败 %d 个", successCount, len(stoppedApps)-successCount)
		
	case "r":
		logger.Info("重启所有应用")
		successCount := 0
		for _, app := range apps {
			logger.Info("重启应用: %s", app.Name)
			if err := appService.RestartApplication(app); err != nil {
				logger.Error("重启应用失败 %s: %v", app.Name, err)
			} else {
				successCount++
			}
		}
		logger.Info("重启完成: 成功 %d 个，失败 %d 个", successCount, len(apps)-successCount)
		
	case "k":
		logger.Info("停止所有运行中的应用")
		runningApps := apps.Running()
		if len(runningApps) == 0 {
			logger.Info("没有运行中的应用")
			return nil
		}
		
		successCount := 0
		for _, app := range runningApps {
			logger.Info("停止应用: %s", app.Name)
			if err := appService.StopApplication(app); err != nil {
				logger.Error("停止应用失败 %s: %v", app.Name, err)
			} else {
				successCount++
			}
		}
		logger.Info("停止完成: 成功 %d 个，失败 %d 个", successCount, len(runningApps)-successCount)
		
	default:
		return fmt.Errorf("未知操作: %s", operation)
	}
	
	return nil
}

// showAllApplicationsStatus 显示所有应用状态
func showAllApplicationsStatus(appService *services.ApplicationService) error {
	// 显示加载提示
	logger.Info("正在扫描应用...")
	
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	logger.Info("正在更新应用状态...")
	
	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}
	
	logger.Title("应用状态概览")
	
	if statusDetail {
		// 显示详细状态
		for i, app := range apps {
			if i > 0 {
				logger.Println("")
			}
			
			status := appService.GetApplicationStatus(app)
			
			showDetailedStatus(status)
		}
	} else {
		// 显示表格状态
		showTableStatus(appService, apps)
	}
	
	// 显示统计信息
	showStatusSummary(apps)
	
	// 显示系统信息
	showSystemInfo()
	
	return nil
}

// showSystemInfo 显示系统信息
func showSystemInfo() {
	systemInfo := config.GetSystemInfo()
	
	logger.Println("")
	logger.Info("系统信息:")
	logger.Printf("  主机名: \033[36m%s\033[0m\n", systemInfo["hostname"])
	logger.Printf("  IP地址: \033[36m%s\033[0m\n", systemInfo["ip"])
	logger.Printf("  项目目录: \033[36m%s\033[0m\n", systemInfo["project_root"])
	logger.Printf("  Nacos地址: \033[36m%s\033[0m\n", systemInfo["nacos_server"])
	logger.Printf("  Nacos命名空间: \033[36m%s\033[0m\n", systemInfo["nacos_namespace"])
	logger.Printf("  JVM参数: \033[36m%s\033[0m\n", systemInfo["jvm_params"])
	logger.Println("")
}

// showDetailedStatus 显示详细状态
func showDetailedStatus(status *models.ApplicationStatus) {
	logger.Printf("应用名称: \033[33m%s\033[0m\n", status.Name)
	logger.Printf("文件路径: %s\n", status.FilePath)
	
	// 状态显示
	statusColor := getStatusColor(status.Status)
	logger.Printf("运行状态: %s%s\033[0m\n", statusColor, status.Status.String())
	
	if status.PID > 0 {
		logger.Printf("进程ID: %d\n", status.PID)
	}
	
	if status.Uptime != "" {
		logger.Printf("运行时长: %s\n", status.Uptime)
	}
	
	if status.CPUUsage != "" {
		logger.Printf("CPU使用率: %s\n", status.CPUUsage)
	}
	
	if status.MemoryUsage != "" {
		logger.Printf("内存使用: %s\n", status.MemoryUsage)
	}
	
	if status.MemorySize != "" {
		logger.Printf("内存大小: %s\n", status.MemorySize)
	}
	
	if status.Port != "" {
		logger.Printf("端口: %s\n", status.Port)
	}
	
	if status.LogFile != "" {
		if _, err := os.Stat(status.LogFile); err == nil {
			logger.Printf("日志文件: \033[32m%s\033[0m\n", status.LogFile)
		} else {
			logger.Printf("日志文件: \033[31m%s (不存在)\033[0m\n", status.LogFile)
		}
	} else {
		logger.Printf("日志文件: \033[31m未配置\033[0m\n")
	}
	
	if status.ConfigFile != "" {
		if _, err := os.Stat(status.ConfigFile); err == nil {
			logger.Printf("配置文件: \033[32m%s\033[0m\n", status.ConfigFile)
		} else {
			logger.Printf("配置文件: \033[31m%s (不存在)\033[0m\n", status.ConfigFile)
		}
	}
}

// showTableStatus 显示表格状态
func showTableStatus(appService *services.ApplicationService, apps models.ApplicationList) {
	// 更新所有应用的状态
	for _, app := range apps {
		app.UpdateStatus()
	}
	
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"序号", "应用名称", "状态", "PID", "运行时长", "CPU", "内存", "内存大小", "端口", "日志"})
	table.SetBorder(true)
	table.SetRowLine(false) // 减少行线以降低表格高度
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColWidth(80) // 设置列宽限制
	
	// 设置颜色
	headerColors := []tablewriter.Colors{
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
	}
	table.SetHeaderColor(headerColors...)
	
	// 统计运行和停止的应用数量，以及总的CPU和内存使用
	runningCount := 0
	stoppedCount := 0
	totalCPU := 0.0
	totalMemoryKB := int64(0)
	totalMemoryPercent := 0.0
	
	for i, app := range apps {
		status := appService.GetApplicationStatus(app)
		
		// 统计状态 - 使用应用的实际运行状态
		if app.IsRunning() {
			runningCount++
			// 获取进程信息用于统计
			info := app.GetProcessInfo()
			totalCPU += info.CPUPercent
			totalMemoryKB += info.MemoryKB
			totalMemoryPercent += info.MemoryPercent
		} else {
			stoppedCount++
		}
		
		// 格式化数据
		pidStr := "-"
		if status.PID > 0 {
			pidStr = fmt.Sprintf("%d", status.PID)
		}
		
		uptimeStr := "--"
		if status.Uptime != "" {
			uptimeStr = status.Uptime
		}
		
		cpuStr := "0%"
		if status.CPUUsage != "" {
			cpuStr = status.CPUUsage
		}
		
		memoryStr := "0%"
		if status.MemoryUsage != "" {
			memoryStr = status.MemoryUsage
		}
		
		memorySizeStr := "0MB"
		if status.MemorySize != "" {
			memorySizeStr = status.MemorySize
		}
		
		portStr := "--"
		if status.Port != "" {
			portStr = status.Port
		}
		
		logFileStr := "✗"
		if status.LogFile != "" {
			if _, err := os.Stat(status.LogFile); err == nil {
				logFileStr = "✓"
			}
		}
		
		// 添加行
		row := []string{
			fmt.Sprintf("%d", i+1),
			status.Name,
			status.Status.String(),
			pidStr,
			uptimeStr,
			cpuStr,
			memoryStr,
			memorySizeStr,
			portStr,
			logFileStr,
		}
		
		// 根据状态设置行颜色
		var rowColors []tablewriter.Colors
		statusColor := getTableStatusColor(status.Status)
		for j := 0; j < len(row); j++ {
			if j == 2 { // 状态列
				rowColors = append(rowColors, statusColor)
			} else {
				rowColors = append(rowColors, tablewriter.Colors{})
			}
		}
		
		table.Rich(row, rowColors)
	}
	
	// 添加合计行
	totalCount := len(apps)
	
	// 格式化合计数据
	totalCPUStr := fmt.Sprintf("%.1f%%", totalCPU)
	totalMemoryPercentStr := fmt.Sprintf("%.1f%%", totalMemoryPercent)
	totalMemorySizeStr := formatMemorySize(totalMemoryKB / 1024) // 转换为MB
	
	summaryRow := []string{
		"-",
		fmt.Sprintf("合计 (%d个应用)", totalCount),
		fmt.Sprintf("运行:%d 停止:%d", runningCount, stoppedCount),
		"-",
		"-",
		totalCPUStr,
		totalMemoryPercentStr,
		totalMemorySizeStr,
		"-",
		"-",
	}
	
	// 设置合计行颜色
	summaryColors := []tablewriter.Colors{
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
	}
	
	table.Rich(summaryRow, summaryColors)
	table.Render()
}

// showStatusSummary 显示状态统计
func showStatusSummary(apps models.ApplicationList) {
	// 在统计前更新所有应用状态
	for _, app := range apps {
		app.UpdateStatus()
	}
	
	runningCount := len(apps.Running())
	stoppedCount := len(apps.Stopped())
	totalCount := len(apps)
	
	logger.Println("")
	logger.Info("状态统计:")
	logger.Printf("  总计: %d 个应用\n", totalCount)
	logger.Printf("  运行中: \033[32m%d\033[0m 个\n", runningCount)
	logger.Printf("  已停止: \033[31m%d\033[0m 个\n", stoppedCount)
	
	// 显示系统内存信息
	showSystemMemoryInfo()
}

// SystemMemoryInfo 系统内存信息
type SystemMemoryInfo struct {
	TotalMB     int64
	UsedMB      int64
	FreeMB      int64
	AvailableMB int64
	SwapTotalMB int64
	SwapUsedMB  int64
	SwapFreeMB  int64
}

// showSystemMemoryInfo 显示系统内存信息
func showSystemMemoryInfo() {
	memInfo := getSystemMemoryInfo()
	if memInfo == nil {
		return
	}
	
	logger.Println("")
	logger.Info("系统内存:")
	
	// 物理内存
	totalStr := formatMemorySize(memInfo.TotalMB)
	usedStr := formatMemorySize(memInfo.UsedMB)
	
	usedPercent := float64(memInfo.UsedMB) / float64(memInfo.TotalMB) * 100
	
	logger.Printf("  物理内存: %s / %s (已用 %.1f%%)", usedStr, totalStr, usedPercent)
	
	// Swap内存
	if memInfo.SwapTotalMB > 0 {
		swapTotalStr := formatMemorySize(memInfo.SwapTotalMB)
		swapUsedStr := formatMemorySize(memInfo.SwapUsedMB)
		swapUsedPercent := float64(memInfo.SwapUsedMB) / float64(memInfo.SwapTotalMB) * 100
		
		logger.Printf("  交换内存: %s / %s (已用 %.1f%%)", swapUsedStr, swapTotalStr, swapUsedPercent)
	}
}

// getSystemMemoryInfo 获取系统内存信息（仅Linux）
func getSystemMemoryInfo() *SystemMemoryInfo {
	return getLinuxMemoryInfo()
}

// getLinuxMemoryInfo 获取Linux系统内存信息
func getLinuxMemoryInfo() *SystemMemoryInfo {
	cmd := exec.Command("free", "-m")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	
	lines := strings.Split(string(output), "\n")
	if len(lines) < 3 {
		return nil
	}
	
	info := &SystemMemoryInfo{}
	
	// 解析内存行
	memFields := strings.Fields(lines[1])
	if len(memFields) >= 7 {
		if total, err := strconv.ParseInt(memFields[1], 10, 64); err == nil {
			info.TotalMB = total
		}
		if used, err := strconv.ParseInt(memFields[2], 10, 64); err == nil {
			info.UsedMB = used
		}
		if free, err := strconv.ParseInt(memFields[3], 10, 64); err == nil {
			info.FreeMB = free
		}
		if available, err := strconv.ParseInt(memFields[6], 10, 64); err == nil {
			info.AvailableMB = available
		}
	}
	
	// 解析Swap行
	if len(lines) >= 3 {
		swapFields := strings.Fields(lines[2])
		if len(swapFields) >= 4 {
			if total, err := strconv.ParseInt(swapFields[1], 10, 64); err == nil {
				info.SwapTotalMB = total
			}
			if used, err := strconv.ParseInt(swapFields[2], 10, 64); err == nil {
				info.SwapUsedMB = used
			}
			if free, err := strconv.ParseInt(swapFields[3], 10, 64); err == nil {
				info.SwapFreeMB = free
			}
		}
	}
	
	return info
}



// formatMemorySize 格式化内存大小（智能单位）
func formatMemorySize(sizeMB int64) string {
	if sizeMB >= 1024 {
		return fmt.Sprintf("%.1fGB", float64(sizeMB)/1024)
	} else {
		return fmt.Sprintf("%dMB", sizeMB)
	}
}

// getStatusColor 获取状态颜色
func getStatusColor(status models.AppStatus) string {
	switch status {
	case models.StatusRunning:
		return "\033[32m" // 绿色
	case models.StatusStopped:
		return "\033[31m" // 红色
	case models.StatusStarting:
		return "\033[33m" // 黄色
	case models.StatusStopping:
		return "\033[33m" // 黄色
	default:
		return "\033[37m" // 白色
	}
}

// getTableStatusColor 获取表格状态颜色
func getTableStatusColor(status models.AppStatus) tablewriter.Colors {
	switch status {
	case models.StatusRunning:
		return tablewriter.Colors{tablewriter.Bold, tablewriter.FgGreenColor}
	case models.StatusStopped:
		return tablewriter.Colors{tablewriter.Bold, tablewriter.FgRedColor}
	case models.StatusStarting, models.StatusStopping:
		return tablewriter.Colors{tablewriter.Bold, tablewriter.FgYellowColor}
	default:
		return tablewriter.Colors{tablewriter.Bold, tablewriter.FgWhiteColor}
	}
}

// showAllApplicationsStatusWithInteractive 显示所有应用状态并进入交互模式
func showAllApplicationsStatusWithInteractive(appService *services.ApplicationService) error {
	// 缓存应用列表，避免重复发现
	var cachedApps models.ApplicationList
	var err error
	
	for {
		// 清屏
		fmt.Print("\033[2J\033[H")
		
		// 只在第一次或用户明确刷新时重新发现应用
		if cachedApps == nil {
			logger.Info("正在扫描应用...")
			cachedApps, err = appService.DiscoverApplications()
			if err != nil {
				return fmt.Errorf("发现应用失败: %w", err)
			}
			logger.Info("正在更新应用状态...")
		}
		
		// 显示应用状态（使用缓存的应用列表）
		if err := showAllApplicationsStatusCached(appService, cachedApps); err != nil {
			return err
		}
		
		// 显示交互帮助
		showInteractiveHelp()
		
		// 等待用户输入
		shouldExit, shouldRefresh := handleInteractiveInputOptimized(appService, cachedApps)
		if shouldExit {
			break
		}
		if shouldRefresh {
			cachedApps = nil // 清空缓存，下次循环重新发现
		}
	}
	
	return nil
}

// showAllApplicationsStatusCached 显示所有应用状态（使用缓存的应用列表）
func showAllApplicationsStatusCached(appService *services.ApplicationService, apps models.ApplicationList) error {
	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return nil
	}
	
	logger.Title("应用状态概览")
	
	if statusDetail {
		// 显示详细状态
		for i, app := range apps {
			if i > 0 {
				logger.Println("")
			}
			
			status := appService.GetApplicationStatus(app)
			
			showDetailedStatus(status)
		}
	} else {
		// 显示表格状态
		showTableStatus(appService, apps)
	}
	
	// 显示统计信息
	showStatusSummary(apps)
	
	// 显示系统信息
	showSystemInfo()
	
	return nil
}

// showInteractiveHelp 显示交互模式帮助
func showInteractiveHelp() {
	logger.Println("")
	logger.Info("交互模式命令:")
	
	// 分成两列显示，减少高度
	fmt.Printf("  \033[33ms<序号>\033[0m - 启动应用 (例: s1)       \033[33mr<序号>\033[0m - 重启应用 (例: r1)\n")
	fmt.Printf("  \033[33mk<序号>\033[0m - 停止应用 (例: k1)       \033[33m9999\033[0m   - 对所有应用执行操作\n")
	fmt.Printf("  \033[33m批量操作\033[0m - 支持逗号和范围 (例: s1,2,3 或 k1-5)\n")
	fmt.Printf("  \033[33m回车\033[0m     - 刷新应用列表         \033[33mq\033[0m      - 退出交互模式\n")
	
	logger.Printf("请输入命令: ")
}

// handleInteractiveInput 处理交互输入
func handleInteractiveInput(appService *services.ApplicationService) bool {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		logger.Error("读取输入失败: %v", err)
		return false
	}
	
	input = strings.TrimSpace(input)
	
	// 空输入，刷新列表
	if input == "" {
		return false
	}
	
	// 退出命令
	if input == "q" || input == "Q" {
		fmt.Print("\033[2J\033[H") // 清屏
		logger.Info("退出交互模式")
		return true
	}
	
	// 解析命令
	if len(input) < 1 {
		logger.Error("无效命令")
		return false
	}
	
	operation := strings.ToLower(string(input[0]))
	if operation != "s" && operation != "r" && operation != "k" {
		logger.Error("无效操作，请使用 s(启动)/r(重启)/k(停止)")
		return false
	}
	
	// 解析序号
	numberStr := strings.TrimSpace(input[1:])
	if numberStr == "" {
		logger.Error("请指定应用序号")
		return false
	}
	
	// 检查是否是批量操作（包含逗号或连字符）
	if strings.Contains(numberStr, ",") || strings.Contains(numberStr, "-") {
		// 批量操作
		if err := executeBatchInteractiveOperation(appService, operation, numberStr); err != nil {
			logger.Error("批量操作失败: %v", err)
		}
	} else {
		// 单个操作
		number, err := strconv.Atoi(numberStr)
		if err != nil {
			logger.Error("无效的序号: %s", numberStr)
			return false
		}
		
		// 执行操作
		if err := executeInteractiveOperation(appService, operation, number); err != nil {
			logger.Error("操作失败: %v", err)
		}
	}
	
	// 等待用户按键继续
	logger.Printf("\n按回车键继续...")
	reader.ReadString('\n')
	
	return false
}

// handleInteractiveInputOptimized 处理交互输入（优化版本）
func handleInteractiveInputOptimized(appService *services.ApplicationService, apps models.ApplicationList) (shouldExit bool, shouldRefresh bool) {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		logger.Error("读取输入失败: %v", err)
		return false, false
	}
	
	input = strings.TrimSpace(input)
	
	// 空输入，刷新列表
	if input == "" {
		return false, false
	}
	
	// 退出命令
	if input == "q" || input == "Q" {
		fmt.Print("\033[2J\033[H") // 清屏
		logger.Info("退出交互模式")
		return true, false
	}
	
	// 刷新命令
	if input == "refresh" || input == "r" {
		return false, true
	}
	
	// 解析命令
	if len(input) < 1 {
		logger.Error("无效命令")
		return false, false
	}
	
	operation := strings.ToLower(string(input[0]))
	if operation != "s" && operation != "r" && operation != "k" {
		logger.Error("无效操作，请使用 s(启动)/r(重启)/k(停止)")
		return false, false
	}
	
	// 解析序号
	numberStr := strings.TrimSpace(input[1:])
	if numberStr == "" {
		logger.Error("请指定应用序号")
		return false, false
	}
	
	// 检查是否是批量操作（包含逗号或连字符）
	if strings.Contains(numberStr, ",") || strings.Contains(numberStr, "-") {
		// 批量操作
		if err := executeBatchInteractiveOperationOptimized(appService, operation, numberStr, apps); err != nil {
			logger.Error("批量操作失败: %v", err)
		}
	} else {
		// 单个操作
		number, err := strconv.Atoi(numberStr)
		if err != nil {
			logger.Error("无效的序号: %s", numberStr)
			return false, false
		}
		
		// 执行操作
		if err := executeInteractiveOperationOptimized(appService, operation, number, apps); err != nil {
			logger.Error("操作失败: %v", err)
		}
	}
	
	// 等待用户按键继续
	logger.Printf("\n按回车键继续...")
	reader.ReadString('\n')
	
	return false, false
}

// executeBatchInteractiveOperation 执行批量交互操作
func executeBatchInteractiveOperation(appService *services.ApplicationService, operation string, numberStr string) error {
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	if len(apps) == 0 {
		return fmt.Errorf("没有找到任何应用")
	}
	
	// 解析批量序号
	numbers, err := services.ParseBatchNumbers(numberStr)
	if err != nil {
		return fmt.Errorf("解析序号失败: %w", err)
	}
	
	// 执行批量操作
	return appService.ExecuteBatchOperation(operation, numbers, apps)
}

// executeBatchInteractiveOperationOptimized 执行批量交互操作（优化版本）
func executeBatchInteractiveOperationOptimized(appService *services.ApplicationService, operation string, numberStr string, apps models.ApplicationList) error {
	if len(apps) == 0 {
		return fmt.Errorf("没有找到任何应用")
	}
	
	// 解析批量序号
	numbers, err := services.ParseBatchNumbers(numberStr)
	if err != nil {
		return fmt.Errorf("解析序号失败: %w", err)
	}
	
	// 执行批量操作
	return appService.ExecuteBatchOperation(operation, numbers, apps)
}

// executeInteractiveOperation 执行交互操作
func executeInteractiveOperation(appService *services.ApplicationService, operation string, number int) error {
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}
	
	if len(apps) == 0 {
		return fmt.Errorf("没有找到任何应用")
	}
	
	// 处理所有应用的情况
	if number == 9999 {
		return executeOperationOnAllApps(appService, apps, operation)
	}
	
	// 检查序号范围
	if number < 1 || number > len(apps) {
		return fmt.Errorf("序号超出范围，应用数量: %d", len(apps))
	}
	
	// 获取指定应用
	app := apps[number-1]
	
	// 执行操作
	switch operation {
	case "s":
		logger.Info("启动应用: %s", app.Name)
		return appService.StartApplication(app)
	case "r":
		logger.Info("重启应用: %s", app.Name)
		return appService.RestartApplication(app)
	case "k":
		logger.Info("停止应用: %s", app.Name)
		return appService.StopApplication(app)
	default:
		return fmt.Errorf("未知操作: %s", operation)
	}
}

// executeInteractiveOperationOptimized 执行交互操作（优化版本）
func executeInteractiveOperationOptimized(appService *services.ApplicationService, operation string, number int, apps models.ApplicationList) error {
	if len(apps) == 0 {
		return fmt.Errorf("没有找到任何应用")
	}
	
	// 处理所有应用的情况
	if number == 9999 {
		return executeOperationOnAllAppsOptimized(appService, apps, operation)
	}
	
	// 检查序号范围
	if number < 1 || number > len(apps) {
		return fmt.Errorf("序号超出范围，应用数量: %d", len(apps))
	}
	
	// 获取指定应用
	app := apps[number-1]
	
	// 执行操作
	switch operation {
	case "s":
		logger.Info("启动应用: %s", app.Name)
		return appService.StartApplication(app)
	case "r":
		logger.Info("重启应用: %s", app.Name)
		return appService.RestartApplication(app)
	case "k":
		logger.Info("停止应用: %s", app.Name)
		return appService.StopApplication(app)
	default:
		return fmt.Errorf("未知操作: %s", operation)
	}
}

// executeOperationOnAllApps 对所有应用执行操作
func executeOperationOnAllApps(appService *services.ApplicationService, apps models.ApplicationList, operation string) error {
	// 在执行操作前，强制更新所有应用的状态
	logger.Info("正在更新应用状态...")
	for _, app := range apps {
		app.UpdateStatus()
	}
	
	switch operation {
	case "s":
		logger.Info("启动所有已停止的应用")
		stoppedApps := apps.Stopped()
		if len(stoppedApps) == 0 {
			logger.Info("没有已停止的应用")
			return nil
		}
		
		successCount := 0
		for _, app := range stoppedApps {
			logger.Info("启动应用: %s", app.Name)
			if err := appService.StartApplication(app); err != nil {
				logger.Error("启动应用失败 %s: %v", app.Name, err)
			} else {
				successCount++
			}
		}
		logger.Info("启动完成: 成功 %d 个，失败 %d 个", successCount, len(stoppedApps)-successCount)
		
	case "r":
		logger.Info("重启所有应用")
		successCount := 0
		for _, app := range apps {
			logger.Info("重启应用: %s", app.Name)
			if err := appService.RestartApplication(app); err != nil {
				logger.Error("重启应用失败 %s: %v", app.Name, err)
			} else {
				successCount++
			}
		}
		logger.Info("重启完成: 成功 %d 个，失败 %d 个", successCount, len(apps)-successCount)
		
	case "k":
		logger.Info("停止所有运行中的应用")
		runningApps := apps.Running()
		if len(runningApps) == 0 {
			logger.Info("没有运行中的应用")
			return nil
		}
		
		successCount := 0
		for _, app := range runningApps {
			logger.Info("停止应用: %s", app.Name)
			if err := appService.StopApplication(app); err != nil {
				logger.Error("停止应用失败 %s: %v", app.Name, err)
			} else {
				successCount++
			}
		}
		logger.Info("停止完成: 成功 %d 个，失败 %d 个", successCount, len(runningApps)-successCount)
		
	default:
		return fmt.Errorf("未知操作: %s", operation)
	}
	
	return nil
}