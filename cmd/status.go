package cmd

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"app-manager-go-v1/internal/services"
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
			// 禁用全局信号处理器，并确保在退出时恢复
			if DisableSignalHandling != nil {
				DisableSignalHandling()
			}
			if EnableSignalHandling != nil {
				defer EnableSignalHandling()
			}

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

	if status.Ports != "" {
		logger.Printf("端口: \033[36m%s\033[0m\n", status.Ports)
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
	// 1. 批量更新所有进程信息以提高性能
	logger.Info("[调试] 步骤1/2: 开始通过 'ps' 命令批量获取所有进程信息...")
	models.BatchUpdateProcessInfo(apps)
	logger.Info("[调试] 步骤1/2: 批量获取进程信息完成。")

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"序号", "应用名称", "状态", "PID", "运行时长", "CPU", "内存", "使用内存", "端口"})
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
	}
	table.SetHeaderColor(headerColors...)

	// 统计变量
	var runningCount, stoppedCount int
	var totalCPU, totalMem float64
	var totalMemSize int64

	for i, app := range apps {
		status := appService.GetApplicationStatus(app) // 从service获取状态

		// 统计状态
		if status.Status == models.StatusRunning {
			runningCount++
			// 累加资源使用情况
			procInfo := app.GetProcessInfo()
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
		avgCPU := totalCPU / float64(runningCount)
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

// showAllApplicationsStatusWithInteractive 显示所有应用状态并进入交互模式
func showAllApplicationsStatusWithInteractive(appService *services.ApplicationService) error {
	// 初始化 readline
	historyFile := filepath.Join(os.TempDir(), "app-manager-history.tmp")
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
                                    - 混合:     1,3-5,8
  其他:                           - 所有:     9999
    q, exit: 退出
    <回车>: 刷新

  示例: s 1,3-5 (启动1,3,4,5)   k 9999 (停止所有)
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
	if operation != "s" && operation != "r" && operation != "k" {
		logger.Error("无效操作，请使用 s(启动)/r(重启)/k(停止)")
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