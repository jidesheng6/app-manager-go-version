package cmd

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"app-manager-go-v1/internal/services"
	"app-manager-go-v1/internal/version"
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	statusDetail bool
	statusJSON   bool
	debugLogs    bool
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
  app-manager status --json           # 以JSON格式输出
  app-manager status --debug-logs     # 打印日志目录诊断信息（软链接/归档状态）`,
	Args: cobra.MaximumNArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		handleError(checkProjectInit(), 1)
	},
	Run: func(cmd *cobra.Command, args []string) {
		// 日志目录诊断（用于排查留痕/软链接问题，只读不修改）
		if debugLogs {
			appName := ""
			if len(args) > 0 {
				appName = args[0]
			}
			handleError(services.PrintLogDiagnostics(appName), 1)
			return
		}

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
	statusCmd.Flags().BoolVar(&debugLogs, "debug-logs", false, "打印日志目录诊断信息（软链接、归档状态）")
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
	showDetailedStatus(status, app)

	return nil
}

// showAllApplicationsStatus 显示所有应用状态
func showAllApplicationsStatus(appService *services.ApplicationService) error {
	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		return fmt.Errorf("发现应用失败: %w", err)
	}

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

			showDetailedStatus(status, app)
		}
	} else {
		// 显示表格状态
		showTableStatus(appService, apps)
	}

	// 显示端口监控表格
	if err := showPortConnectionsTable(appService); err != nil {
		logger.Warning("显示端口监控表格失败: %v", err)
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
	
	logger.Printf("\n\n")
	logger.Info("系统信息:")
	logger.Printf("主机名: \033[33m%s\033[0m\n", systemInfo["hostname"])
	logger.Printf("IP地址: \033[36m%s\033[0m\n", systemInfo["ip"])
	logger.Printf("CPU核心: \033[33m%s\033[0m (CPU使用率按此核数归一化)\n", systemInfo["cpu_count"])
	logger.Printf("版本: \033[35m%s\033[0m\n", version.BuildLine(Version))
	logger.Printf("代码版本: %s\n", version.RepoLine(version.Live(config.GetProjectRoot())))
	if hint := version.DriftHint(version.Embedded(), version.Live(config.GetProjectRoot())); hint != "" {
		logger.Printf("\033[33m提示: %s\033[0m\n", hint)
	}
	logger.Printf("项目目录: \033[32m%s\033[0m\n", systemInfo["project_root"])
	logger.Printf("Nacos地址: \033[35m%s\033[0m\n", systemInfo["nacos_server"])
	logger.Printf("Nacos命名空间: \033[34m%s\033[0m\n", systemInfo["nacos_namespace"])
	logger.Printf("JVM参数: \033[37m%s\033[0m\n", systemInfo["jvm_params"])
	logger.Println("")
}

// showDetailedStatus 显示详细状态
func showDetailedStatus(status *models.ApplicationStatus, app *models.Application) {
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
		logger.Printf("CPU使用率: \033[33m%s\033[0m  \033[90m(整机口径 / %d 核)\033[0m\n",
			status.CPUUsage, models.CPUCount())
	}

	if status.MemoryUsage != "" {
		logger.Printf("内存使用: %s\n", status.MemoryUsage)
	}

	if status.MemorySize != "" {
		logger.Printf("内存大小: %s\n", status.MemorySize)
	}

	if status.Ports != "" {
		logger.Printf("端口: \033[36m%s\033[0m\n", status.Ports)
	}

	// 显示监控信息（如果可用）
	if app != nil {
		procInfo := app.GetProcessInfo()
		
		// 磁盘 I/O 信息
		if procInfo.DiskReadBytes > 0 || procInfo.DiskWriteBytes > 0 {
			logger.Printf("磁盘 I/O: 读取 %s, 写入 %s\n", 
				formatBytes(int64(procInfo.DiskReadBytes)), 
				formatBytes(int64(procInfo.DiskWriteBytes)))
		}
		
		if procInfo.DiskReadRate != "" || procInfo.DiskWriteRate != "" {
			logger.Printf("磁盘速率: 读取 %s, 写入 %s\n", 
				getStringOrDefault(procInfo.DiskReadRate, "0 B/s"), 
				getStringOrDefault(procInfo.DiskWriteRate, "0 B/s"))
		}
		
		// 网络统计信息
		if procInfo.NetworkRxBytes > 0 || procInfo.NetworkTxBytes > 0 {
			logger.Printf("网络流量: 接收 %s, 发送 %s\n", 
				formatBytes(int64(procInfo.NetworkRxBytes)), 
				formatBytes(int64(procInfo.NetworkTxBytes)))
		}
		
		if procInfo.NetworkRxRate != "" || procInfo.NetworkTxRate != "" {
			logger.Printf("网络速率: 接收 %s, 发送 %s\n", 
				getStringOrDefault(procInfo.NetworkRxRate, "0 B/s"), 
				getStringOrDefault(procInfo.NetworkTxRate, "0 B/s"))
		}
		
		// 端口连接状态
		if procInfo.PortConnections != "" {
			logger.Printf("端口连接状态: %s\n", procInfo.PortConnections)
		}
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
func showTableStatus(appService *services.ApplicationService, apps models.ApplicationList) error {
	// 1. 批量更新所有进程信息以提高性能（包含监控数据）
	logger.Info("[调试] 步骤1/2: 开始通过 'ps' 命令批量获取所有进程信息...")
	appService.BatchUpdateProcessInfoWithMonitoring(apps)
	logger.Info("[调试] 步骤1/2: 批量获取进程信息完成。")

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"序号", "应用名称", "状态", "PID", "运行时长", "CPU", "内存", "使用内存", "磁盘读", "磁盘写", "网络收", "网络发", "端口"})
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
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
	}
	table.SetHeaderColor(headerColors...)

	// 统计变量
	var runningCount, stoppedCount int
	var totalCPU, totalMem float64
	var totalMemSize int64

	for i, app := range apps {
		status := appService.GetApplicationStatus(app) // 从service获取状态

		// 进程信息（CPU/内存/IO/网络都从这里取）
		procInfo := app.GetProcessInfo()
		if procInfo == nil {
			procInfo = &models.ProcessInfo{}
		}

		// 统计状态
		if status.Status == models.StatusRunning {
			runningCount++
			// 累加资源使用情况（CPU 用单核口径累加，最后统一换算整机口径）
			totalCPU += procInfo.CPUPercent
			totalMem += procInfo.MemoryPercent
			totalMemSize += procInfo.MemoryKB
		} else if status.Status == models.StatusStopped {
			stoppedCount++
		}

		// 格式化数据
		pidStr := "-"
		if status.PID > 0 {
			pidStr = fmt.Sprintf("%d", status.PID)
		}

		portsStr := "--"
		if status.Ports != "" {
			portsStr = status.Ports
		}

		uptimeStr := "--"
		if status.Uptime != "" {
			uptimeStr = status.Uptime
		}

		// CPU：单核口径超过 100% 时同时标注整机口径
		cpuStr := "0.0%"
		if procInfo.CPUPercent > 0 {
			cpuStr = models.FormatCPUWithHost(procInfo.CPUPercent)
		}

		memoryStr := "0%"
		if status.MemoryUsage != "" {
			memoryStr = status.MemoryUsage
		}

		memorySizeStr := "0MB"
		if status.MemorySize != "" {
			memorySizeStr = status.MemorySize
		}

		// 磁盘 I/O 信息
		diskReadStr := "--"
		diskWriteStr := "--"
		if procInfo.DiskReadRate != "" {
			diskReadStr = procInfo.DiskReadRate
		}
		if procInfo.DiskWriteRate != "" {
			diskWriteStr = procInfo.DiskWriteRate
		}
		
		// 网络统计信息
		networkRxStr := "--"
		networkTxStr := "--"
		if procInfo.NetworkRxRate != "" {
			networkRxStr = procInfo.NetworkRxRate
		}
		if procInfo.NetworkTxRate != "" {
			networkTxStr = procInfo.NetworkTxRate
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
			diskReadStr,
			diskWriteStr,
			networkRxStr,
			networkTxStr,
			portsStr,
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

	avgCPUStr := "0.0%"
	avgMemStr := "0.0%"
	if runningCount > 0 {
		// CPU 换算成整机口径再求平均，保证 0~100% 且可跨应用理解
		avgCPU := models.CPUPercentOfHost(totalCPU) / float64(runningCount)
		avgMem := totalMem / float64(runningCount)

		avgCPUStr = fmt.Sprintf("Avg %.1f%%", avgCPU)
		avgMemStr = fmt.Sprintf("Avg %.1f%%", avgMem)
	}

	// 格式化总内存大小
	totalMemSizeStr := formatMemorySizeKB(totalMemSize)

	summaryRow := []string{
		"-",
		fmt.Sprintf("合计 (%d个应用)", totalCount),
		fmt.Sprintf("运行:%d 停止:%d", runningCount, stoppedCount),
		"-",
		"-",
		avgCPUStr,
		avgMemStr,
		totalMemSizeStr,
		"-",
		"-",
		"-",
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
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
		{tablewriter.Bold, tablewriter.FgYellowColor},
	}

	table.Rich(summaryRow, summaryColors)
	table.Render()

	return nil
}

// showPortConnectionsTable 显示独立的端口监控表格
func showPortConnectionsTable(appService *services.ApplicationService) error {
	// 检查是否启用端口监控
	if !config.AppConfig.Monitoring.EnablePortStats {
		return nil
	}

	// 获取配置的监控端口列表
	monitorPorts := config.AppConfig.Monitoring.MonitorPorts
	openRestyPort := config.AppConfig.Monitoring.OpenRestyPort

	// 如果没有配置任何端口，则不显示表格
	if monitorPorts == "" && openRestyPort == 0 {
		return nil
	}

	logger.Println("")
	logger.Title("端口监控状态")

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"端口", "协议", "状态", "连接数", "唯一IP", "类型", "说明"})
	table.SetBorder(true)
	table.SetRowLine(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColWidth(80)

	// 设置表头颜色
	headerColors := []tablewriter.Colors{
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
		{tablewriter.Bold, tablewriter.FgCyanColor},
	}
	table.SetHeaderColor(headerColors...)

	var totalConnections int

	// 处理配置的监控端口列表
	if monitorPorts != "" {
		ports := strings.Split(monitorPorts, ",")
		for _, portStr := range ports {
			portStr = strings.TrimSpace(portStr)
			if portStr == "" {
				continue
			}

			port, err := strconv.Atoi(portStr)
			if err != nil {
				logger.Warning("无效的端口号: %s", portStr)
				continue
			}

			// 获取端口连接统计
			connections, err := appService.GetPortConnectionStats(port)
			if err != nil {
				logger.Warning("获取端口 %d 连接统计失败: %v", port, err)
				continue
			}

			status := "关闭"
			statusColor := tablewriter.Colors{tablewriter.FgRedColor}
			connCount := 0
			uniqueIPCount := 0
			if connections != nil && connections.IsListening {
				status = "监听"
				statusColor = tablewriter.Colors{tablewriter.FgGreenColor}
				connCount = connections.Connections
				uniqueIPCount = connections.UniqueIPs
				totalConnections += connections.Connections
			}

			row := []string{
				fmt.Sprintf("%d", port),
				"TCP",
				status,
				fmt.Sprintf("%d", connCount),
				fmt.Sprintf("%d", uniqueIPCount),
				"通用",
				"配置监控端口",
			}

			// 设置行颜色
			rowColors := []tablewriter.Colors{
				{},
				{},
				statusColor,
				{},
				{},
				{},
				{},
			}

			table.Rich(row, rowColors)
		}
	}

	// 处理OpenResty端口
	if openRestyPort > 0 {
		connections, err := appService.GetPortConnectionStats(openRestyPort)
		if err != nil {
			logger.Warning("获取OpenResty端口 %d 连接统计失败: %v", openRestyPort, err)
		} else {
			status := "关闭"
			statusColor := tablewriter.Colors{tablewriter.FgRedColor}
			connCount := 0
			uniqueIPCount := 0
			if connections != nil && connections.IsListening {
				status = "监听"
				statusColor = tablewriter.Colors{tablewriter.FgGreenColor}
				connCount = connections.Connections
				uniqueIPCount = connections.UniqueIPs
				totalConnections += connections.Connections
			}

			row := []string{
				fmt.Sprintf("%d", openRestyPort),
				"HTTP",
				status,
				fmt.Sprintf("%d", connCount),
				fmt.Sprintf("%d", uniqueIPCount),
				"OpenResty",
				"前端在线人数",
			}

			// 设置行颜色
			rowColors := []tablewriter.Colors{
				{},
				{},
				statusColor,
				{},
				{},
				{tablewriter.Bold, tablewriter.FgYellowColor},
				{tablewriter.Bold, tablewriter.FgYellowColor},
			}

			table.Rich(row, rowColors)
		}
	}

	// 添加合计行
	summaryRow := []string{
		"-",
		"-",
		"-",
		fmt.Sprintf("总计: %d", totalConnections),
		"-",
		"-",
		"端口监控统计",
	}

	summaryColors := []tablewriter.Colors{
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

	return nil
}

// showStatusSummary 显示状态统计
func showStatusSummary(apps models.ApplicationList) {
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

// getSystemMemoryInfo 获取系统内存信息
func getSystemMemoryInfo() *SystemMemoryInfo {
	return getLinuxMemoryInfo()
}

// formatMemorySizeKB 格式化内存大小（智能单位，输入为KB）
func formatMemorySizeKB(sizeKB int64) string {
	if sizeKB < 1024 {
		return fmt.Sprintf("%d KB", sizeKB)
	}
	sizeMB := float64(sizeKB) / 1024
	if sizeMB < 1024 {
		return fmt.Sprintf("%.1f MB", sizeMB)
	}
	sizeGB := sizeMB / 1024
	return fmt.Sprintf("%.1f GB", sizeGB)
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

// getStringOrDefault 返回字符串或默认值
func getStringOrDefault(str, defaultValue string) string {
	if str == "" {
		return defaultValue
	}
	return str
}

// formatBytes 格式化字节数为可读格式
func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	
	units := []string{"B", "KB", "MB", "GB", "TB"}
	size := float64(bytes)
	unitIndex := 0
	
	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}
	
	if unitIndex == 0 {
		return fmt.Sprintf("%.0f %s", size, units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", size, units[unitIndex])
}

// showAllApplicationsStatusWithInteractive 显示所有应用状态并进入交互模式
func showAllApplicationsStatusWithInteractive(appService *services.ApplicationService) error {
	// 初始化 readline
	historyFile := filepath.Join(config.GetTempDir(), "app-manager-history.tmp")
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[36m[交互模式]\033[0m 请输入命令 > ",
		HistoryFile:     historyFile,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		return fmt.Errorf("初始化Readline失败: %w", err)
	}
	defer rl.Close()

	for {
		// 清屏
		fmt.Print("\033[2J\033[H")

		// 显示应用状态
		if err := showAllApplicationsStatus(appService); err != nil {
			logger.Error("刷新应用状态失败: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// 显示交互帮助
		showInteractiveHelp()

		// 等待用户输入
		if shouldExit := handleInteractiveInput(appService, rl); shouldExit {
			break
		}
	}

	return nil
}

// showInteractiveHelp 显示交互模式帮助
func showInteractiveHelp() {
	logger.Println("")
	logger.Info("交互模式命令:")

	helpText := `
  操作:                           序号格式 (可组合):
    s: 启动                         - 单个:     1
    k: 停止                         - 多个:     1,3,5
    r: 重启                         - 范围:     1-5
    L: 查看日志                     - 混合:     1,3-5,8
                                    - 所有:     9999
  其他:
    q, exit: 退出
    <回车>: 刷新

  示例: s 1,3-5 (启动1,3,4,5)   L1 500 (查看应用1的500行日志)
`
	fmt.Println(helpText)
}

// handleInteractiveInput 处理交互输入
func handleInteractiveInput(appService *services.ApplicationService, rl *readline.Instance) bool {
	line, err := rl.Readline()

	if err == readline.ErrInterrupt { // Ctrl+C
		return true // 退出
	} else if err == io.EOF { // Ctrl+D
		return true // 退出
	}

	input := strings.TrimSpace(line)

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
	
	// 处理日志查看命令
	if operation == "l" {
		// 发现应用
		apps, err := appService.DiscoverApplications()
		if err != nil {
			logger.Error("发现应用失败: %v", err)
			time.Sleep(1 * time.Second)
			return false
		}
		
		if err := handleLogCommand(input, apps); err != nil {
			logger.Error("查看日志失败: %v", err)
			time.Sleep(1 * time.Second)
		}
		
		// 等待用户按键继续
		logger.Printf("\n按回车键继续...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return false
	}
	
	if operation != "s" && operation != "r" && operation != "k" {
		logger.Error("无效操作，请使用 s(启动)/r(重启)/k(停止)/L(查看日志)")
		time.Sleep(1 * time.Second)
		return false
	}

	// 解析序号
	numberStr := strings.TrimSpace(input[1:])
	if numberStr == "" {
		logger.Error("请指定应用序号")
		time.Sleep(1 * time.Second)
		return false
	}

	// 发现应用
	apps, err := appService.DiscoverApplications()
	if err != nil {
		logger.Error("发现应用失败: %v", err)
		return false
	}

	if len(apps) == 0 {
		logger.Info("没有找到任何应用")
		return false
	}

	var targetApps models.ApplicationList

	if strings.ToLower(numberStr) == "9999" {
		targetApps = apps
	} else {
		numbers, err := parseNumberRanges(numberStr)
		if err != nil {
			logger.Error("无效的序号格式: %v", err)
			time.Sleep(1 * time.Second)
			return false
		}

		for _, num := range numbers {
			if num < 1 || num > len(apps) {
				logger.Error("序号 %d 超出范围 (1-%d)", num, len(apps))
				time.Sleep(1 * time.Second)
				return false
			}
			targetApps = append(targetApps, apps[num-1])
		}
	}

	if len(targetApps) == 0 {
		logger.Error("没有匹配到任何应用")
		time.Sleep(1 * time.Second)
		return false
	}

	// 仅在批量启动或重启时按优先级排序
	if (operation == "s" || operation == "r") && strings.ToLower(numberStr) == "9999" {
		logger.Info("批量操作，按优先级排序...")
		targetApps.SortByPriority()
	}

	executeInteractiveOperation(appService, operation, targetApps)

	// 等待用户按键继续
	logger.Printf("\n按回车键继续...")
	// 等待用户按任意键继续，这里用一个新的reader，避免与readline冲突
	bufio.NewReader(os.Stdin).ReadString('\n')

	return false
}

// executeInteractiveOperation 执行交互操作
func executeInteractiveOperation(appService *services.ApplicationService, operation string, apps models.ApplicationList) {
	for _, app := range apps {
		switch operation {
		case "s":
			logger.Title("启动应用: %s", app.Name)
			if !app.IsRunning() {
				err := appService.StartApplication(app)
				if err != nil {
					logger.Error("启动应用 %s 失败: %v", app.Name, err)
				} else {
					if app.StartupDelay > 0 {
						logger.Info("等待 %d 秒后继续...", app.StartupDelay)
						time.Sleep(time.Duration(app.StartupDelay) * time.Second)
					}
				}
			} else {
				logger.Info("应用 %s 已在运行，跳过启动", app.Name)
			}
		case "r":
			logger.Title("重启应用: %s", app.Name)
			err := appService.RestartApplication(app)
			if err != nil {
				logger.Error("重启应用 %s 失败: %v", app.Name, err)
			} else {
				if app.StartupDelay > 0 {
					logger.Info("等待 %d 秒后继续...", app.StartupDelay)
					time.Sleep(time.Duration(app.StartupDelay) * time.Second)
				}
			}
		case "k":
			logger.Title("停止应用: %s", app.Name)
			if app.IsRunning() {
				logger.Info("-> 停止: %s", app.Name)
				if err := appService.StopApplication(app); err != nil {
					logger.Error("   停止失败: %v", err)
				}
			} else {
				logger.Warning("-> 跳过: %s (已停止)", app.Name)
			}
		}
	}
}

// parseLogCommand 解析 L 命令的应用索引和行数参数
// 支持格式: L1, L2, L1 900, L2 500 等
func parseLogCommand(input string) (appIndex int, lines int, err error) {
	// 默认行数
	lines = 500
	
	// 移除 L 前缀
	if len(input) < 2 || strings.ToLower(string(input[0])) != "l" {
		return 0, 0, fmt.Errorf("无效的 L 命令格式")
	}
	
	// 解析剩余部分
	remaining := strings.TrimSpace(input[1:])
	if remaining == "" {
		return 0, 0, fmt.Errorf("请指定应用序号，如: L1")
	}
	
	// 按空格分割
	parts := strings.Fields(remaining)
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("请指定应用序号，如: L1")
	}
	
	// 解析应用索引
	appIndex, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("无效的应用序号: %s", parts[0])
	}
	
	// 解析行数（可选）
	if len(parts) > 1 {
		lines, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("无效的行数: %s", parts[1])
		}
		if lines <= 0 {
			return 0, 0, fmt.Errorf("行数必须大于0")
		}
	}
	
	return appIndex, lines, nil
}

// handleLogCommand 处理日志查看命令
func handleLogCommand(input string, apps models.ApplicationList) error {
	// 解析命令参数
	appIndex, lines, err := parseLogCommand(input)
	if err != nil {
		return err
	}
	
	// 验证应用索引
	if appIndex < 1 || appIndex > len(apps) {
		return fmt.Errorf("应用序号 %d 超出范围 (1-%d)", appIndex, len(apps))
	}
	
	// 获取目标应用
	app := apps[appIndex-1]

	// 解析当前日志（会话日志优先，兼容旧命名）
	logPath := services.ResolveCurrentLog(app.Name)
	if logPath == "" {
		logPath = app.LogFile
	}

	// 检查日志文件配置
	if logPath == "" {
		return fmt.Errorf("应用 %s 没有配置日志文件", app.Name)
	}
	
	// 检查日志文件是否存在
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("日志文件不存在: %s", logPath)
	}
	
	// 清屏并显示日志
	fmt.Print("\033[2J\033[H")
	logger.Title("查看应用日志")
	logger.Info("应用名称: %s", app.Name)
	logger.Info("日志文件: %s", logPath)
	logger.Info("显示行数: %d", lines)
	logger.Println("")
	
	// 读取日志文件
	err = readLogTail(logPath, lines)
	if err != nil {
		return fmt.Errorf("读取日志失败: %w", err)
	}
	
	logger.Println("")
	logger.Info("按回车键返回状态界面...")
	
	return nil
}

// readLogTail 读取日志文件的最后几行
func readLogTail(logFile string, lines int) error {
	file, err := os.Open(logFile)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer file.Close()
	
	// 读取所有行
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取日志文件失败: %w", err)
	}
	
	// 显示最后几行
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	
	for i := start; i < len(allLines); i++ {
		printLogLine(allLines[i])
	}
	
	return nil
}

// printLogLine 打印日志行（带颜色格式化）
func printLogLine(line string) {
	// 简单的日志级别颜色化
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

// parseNumberRanges 解析用户输入的数字范围字符串
func parseNumberRanges(rangeStr string) ([]int, error) {
	var numbers []int
	// 使用集合来自动处理重复的数字
	numSet := make(map[int]bool)

	// 1. 按逗号分割字符串
	parts := strings.Split(rangeStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 2. 检查是否是范围 (如 "1-5")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("无效的范围格式: '%s'", part)
			}

			start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))

			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("范围中的数字无效: '%s'", part)
			}

			if start > end {
				start, end = end, start // 自动交换，支持 "5-1" 这种格式
			}

			for i := start; i <= end; i++ {
				numSet[i] = true
			}
		} else {
			// 3. 处理单个数字
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("无效的数字: '%s'", part)
			}
			numSet[num] = true
		}
	}

	// 4. 将集合转换为切片
	for num := range numSet {
		numbers = append(numbers, num)
	}

	// 5. 对结果进行排序，便于后续处理
	sort.Ints(numbers)

	return numbers, nil
}

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}