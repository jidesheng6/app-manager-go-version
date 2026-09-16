package services

import (
	"app-manager-go-v1/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// 应用日志留痕（会话日志）命名规则：
//
//	本次运行   : <app>-<启动时间>.log                例如 party-auth-2025-01-01-101530.log
//	已归档会话 : <app>-<启动时间>-停止.log           例如 party-auth-2025-01-01-101530-停止.log
//	当前快捷   : <app>.log （软链接，指向最新一次运行/最近归档的日志；Windows 不支持则自动跳过）
//
// 说明：日志按“运行周期”分段，每次 start 新建一个文件，stop/restart 时把当前文件改名归档，
// 因此历史上任何一次启动的输出都不会被覆盖，程序被 kill -9 时也会在下次启动前补归档。

const (
	// SessionTimeLayout 会话日志文件名中的时间格式: yyyy-MM-dd-HHmmss
	SessionTimeLayout = "2006-01-02-150405"

	// stoppedSuffix 归档标记，插在 .log 之前
	stoppedSuffix = "-停止"

	// logExt 会话日志扩展名
	logExt = ".log"
)

var (
	// sessionStartRegex 匹配会话/归档命名中的 <app>-<yyyy-MM-dd-HHmmss> 前缀
	sessionStartRegex = regexp.MustCompile(`^(.*)-\d{4}-\d{2}-\d{2}-\d{6}(-停止)?\.log$`)
	// sessionLogRegex 匹配 <app>-<yyyy-MM-dd-HHmmss>.log
	sessionLogRegex = regexp.MustCompile(`^(.*)-(\d{4}-\d{2}-\d{2}-\d{6})\.log$`)
	// stoppedLogRegex 匹配 <app>-<yyyy-MM-dd-HHmmss>-停止.log
	stoppedLogRegex = regexp.MustCompile(`^(.*)-(\d{4}-\d{2}-\d{2}-\d{6})-停止\.log$`)
)

// LogSession 表示一次运行周期产生的日志文件
type LogSession struct {
	AppName   string    // 应用名称
	Path      string    // 日志文件完整路径
	StartTime time.Time // 会话启动时间（文件名中的时间）
	Stopped   bool      // 是否已归档（文件名带 -停止）
	Size      int64     // 文件大小(字节)
	ModTime   time.Time // 最后修改时间
}

// LegacyLogPath 返回旧版固定日志路径 <logs>/<app>.log
func LegacyLogPath(appName string) string {
	return filepath.Join(config.GetLogsDir(), appName+logExt)
}

// CurrentLogLinkPath 返回“当前日志”快捷软链接路径 <logs>/<app>.log
func CurrentLogLinkPath(appName string) string {
	return LegacyLogPath(appName)
}

// NewSessionLogPath 生成某个启动时刻对应的会话日志路径
func NewSessionLogPath(appName string, start time.Time) string {
	return filepath.Join(config.GetLogsDir(), appName+"-"+start.Format(SessionTimeLayout)+logExt)
}

// hasStoppedMark 判断文件名（含 .log）是否带归档标记
func hasStoppedMark(fileName string) bool {
	return strings.HasSuffix(strings.TrimSuffix(fileName, logExt), stoppedSuffix)
}

// sessionAppName 从会话日志文件名中还原应用名称
func sessionAppName(fileName string) string {
	base := filepath.Base(fileName)

	// 会话/归档命名: 去掉 -yyyy-MM-dd-HHmmss 与 -停止 后缀
	if m := sessionStartRegex.FindStringSubmatch(base); m != nil {
		return m[1]
	}

	// 其他命名（例如旧版 <app>.log）: 只去扩展名
	return strings.TrimSuffix(base, logExt)
}

// parseSessionTime 解析文件名中的时间片段，失败时返回零值
func parseSessionTime(value string) time.Time {
	parsed, err := time.ParseInLocation(SessionTimeLayout, value, time.Local)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// archivedSessionPath 生成归档后的日志路径: <app>-<时间>-停止.log
// filePath 为当前会话日志；fallbackTime 用于无法从文件名解析启动时间时的时间戳
func archivedSessionPath(filePath string, fallbackTime time.Time) string {
	appName := sessionAppName(filePath)

	start := parseSessionTime(sessionStartSegment(filepath.Base(filePath)))
	if start.IsZero() {
		start = fallbackTime
	}
	if start.IsZero() {
		start = time.Now()
	}

	return filepath.Join(filepath.Dir(filePath), appName+"-"+start.Format(SessionTimeLayout)+stoppedSuffix+logExt)
}

// sessionStartSegment 从会话日志文件名中取出 -yyyy-MM-dd-HHmmss 时间片段
func sessionStartSegment(fileName string) string {
	if m := sessionLogRegex.FindStringSubmatch(filepath.Base(fileName)); m != nil {
		return m[2]
	}
	if m := stoppedLogRegex.FindStringSubmatch(filepath.Base(fileName)); m != nil {
		return m[2]
	}
	return ""
}

// parseSessionName 解析会话日志文件名，返回应用名称、启动时间和归档标记
// 非会话日志命名（例如旧版 <app>.log）返回 stopped=false、时间零值
func parseSessionName(fileName string) (appName string, start time.Time, stopped bool) {
	base := filepath.Base(fileName)

	// 归档命名 <app>-<yyyy-MM-dd-HHmmss>-停止.log
	if m := stoppedLogRegex.FindStringSubmatch(base); m != nil {
		return m[1], parseSessionTime(m[2]), true
	}

	// 会话命名 <app>-<yyyy-MM-dd-HHmmss>.log
	if m := sessionLogRegex.FindStringSubmatch(base); m != nil {
		return m[1], parseSessionTime(m[2]), false
	}

	return strings.TrimSuffix(base, logExt), time.Time{}, false
}

// resolveRealLogPath 解析日志真实文件路径：
// 软链接取其目标（目标不存在时返回空字符串），普通文件原样返回
func resolveRealLogPath(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}

	// 不是软链接：普通文件直接用
	if info.Mode()&os.ModeSymlink == 0 {
		return path
	}

	if _, err := os.Stat(path); err != nil {
		return "" // 悬空链接
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// isRegularLogFile 判断路径是否为可归档的普通日志文件
func isRegularLogFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Size() > 0
}

// nextSessionLogPath 生成本次会话日志路径
// 同一秒内重复启动时把时间戳顺延 1 秒，保证文件名始终符合 <app>-<yyyy-MM-dd-HHmmss>.log
func nextSessionLogPath(appName string, start time.Time) string {
	for i := 0; i < 60; i++ {
		candidate := NewSessionLogPath(appName, start.Add(time.Duration(i)*time.Second))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return NewSessionLogPath(appName, time.Now().Add(time.Minute))
}

// cleanupStraySessionLinks 清理历史版本遗留的日志软链接
// （会话日志、归档日志都应该是真实文件，出现软链接一定是旧版本的脏数据）
func cleanupStraySessionLinks(appName string) int {
	logsDir := config.GetLogsDir()
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		name := entry.Name()
		// <app>.log 是运行期链接，这里不动
		if name == appName+logExt {
			continue
		}
		// 只清理本应用的会话/归档命名链接，不动其他应用和用户自己建的文件
		if parsedName, _, _ := parseSessionName(name); parsedName != appName {
			continue
		}

		if err := os.Remove(filepath.Join(logsDir, name)); err == nil {
			removed++
		}
	}

	return removed
}

// nextArchivedPath 生成不与现有文件冲突的归档路径
func nextArchivedPath(path string) string {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return fmt.Sprintf("%s(%d)%s", base, time.Now().UnixNano(), ext)
}

// archiveLogFile 把正在写入的日志文件改名为归档文件 <app>-<启动时间>-停止.log
//
// 只接受普通文件：软链接必须先由调用方解析成真实路径，
// 否则 os.Rename 会重命名软链接本身（线上曾因此滚出 -停止(2).log -> -停止.log 的链子）。
func archiveLogFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("拒绝直接归档软链接（应先解析真实路径）: %s", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("日志路径是目录: %s", path)
	}

	// 已归档的文件不再改名
	if hasStoppedMark(filepath.Base(path)) {
		return path, nil
	}

	// 内容最后写入时间作为兜底时间戳，保证归档名唯一
	stopTime := info.ModTime()
	if stopTime.IsZero() {
		stopTime = time.Now()
	}

	target := nextArchivedPath(archivedSessionPath(path, stopTime))
	if filepath.Clean(target) == filepath.Clean(path) {
		return path, nil
	}

	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	return target, nil
}

// ArchiveLegacyLog 归档旧命名(<app>.log，真实文件，非软链接)的日志
// 返回归档后的路径，没有旧日志时返回空字符串
func ArchiveLegacyLog(appName string) (string, error) {
	legacy := LegacyLogPath(appName)

	info, err := os.Lstat(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil // 软链接不是真实日志，交给调用方重建
	}
	if info.Size() == 0 {
		return "", nil // 上一次启动没有产生输出，直接复用旧文件
	}

	archived, err := archiveLogFile(legacy)
	if err != nil {
		return "", err
	}

	// 旧文件已归档，移除指向它的软链接
	removeCurrentLogLink(appName)
	return archived, nil
}

// ResolveCurrentLog 解析应用“当前”使用的日志文件：
//  1. <app>.log（运行期软链接，或旧版遗留的真实文件）
//  2. 最新一次未归档的会话日志
//  3. 最近一次归档日志（应用已停止时，让它指向最后一次运行的输出）
//
// 找不到时返回空字符串
func ResolveCurrentLog(appName string) string {
	linkPath := CurrentLogLinkPath(appName)

	if info, err := os.Lstat(linkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Stat(linkPath); err == nil && !target.IsDir() {
				return linkPath
			}
		} else if !info.IsDir() {
			return linkPath
		}
	}

	if newest := newestActiveSessionLog(appName); newest != nil {
		return newest.Path
	}

	if newest := newestSessionLog(appName); newest != nil {
		return newest.Path
	}

	return ""
}

// linkCurrentLog 建立运行期软链接 <app>.log -> 本次会话日志
// 拒绝指向归档文件：软链接只在应用运行期间有意义，停止后由 removeCurrentLogLink 清理
func linkCurrentLog(appName, target string) {
	if target == "" || hasStoppedMark(filepath.Base(target)) {
		return
	}

	linkPath := CurrentLogLinkPath(appName)
	if filepath.Clean(linkPath) == filepath.Clean(target) {
		return
	}

	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink == 0 {
		// 旧命名真实文件仍在，不能覆盖
		return
	}

	_ = os.Remove(linkPath)
	if err := os.Symlink(filepath.Base(target), linkPath); err != nil {
		return // Windows 无权限等情况，忽略
	}
}

// removeCurrentLogLink 删除旧的“当前日志”软链接
func removeCurrentLogLink(appName string) {
	linkPath := CurrentLogLinkPath(appName)
	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(linkPath)
	}
}

// ListLogSessions 列出某应用的全部日志会话（新的在前）
func ListLogSessions(appName string) []*LogSession {
	logsDir := config.GetLogsDir()
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil
	}

	var sessions []*LogSession
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name, start, stopped := parseSessionName(entry.Name())
		if name != appName || start.IsZero() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		sessions = append(sessions, &LogSession{
			AppName:   name,
			Path:      filepath.Join(logsDir, entry.Name()),
			StartTime: start,
			Stopped:   stopped,
			Size:      info.Size(),
			ModTime:   info.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions
}

// CountLogSessions 统计应用的日志会话数量
func CountLogSessions(appName string) int {
	return len(ListLogSessions(appName))
}

// newestSessionLog 返回最新的日志会话（含已归档），没有会话日志时返回 nil
func newestSessionLog(appName string) *LogSession {
	sessions := ListLogSessions(appName)
	if len(sessions) == 0 {
		return nil
	}
	return sessions[0]
}

// newestActiveSessionLog 返回最新的未归档会话（即上一次运行还开着的日志）
func newestActiveSessionLog(appName string) *LogSession {
	for _, session := range ListLogSessions(appName) {
		if !session.Stopped {
			return session
		}
	}
	return nil
}

// PrintLogDiagnostics 打印日志目录诊断信息（只读，用于排查留痕/软链接问题）
func PrintLogDiagnostics(appName string) error {
	logsDir := config.GetLogsDir()
	fmt.Printf("日志目录: %s\n", logsDir)

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	fmt.Printf("目录项: %d\n\n", len(entries))

	links, files := 0, 0
	for _, entry := range entries {
		fullPath := filepath.Join(logsDir, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			fmt.Printf("  %-70s [读取失败: %v]\n", entry.Name(), err)
			continue
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			links++
			target, linkErr := os.Readlink(fullPath)
			if linkErr != nil {
				target = "[读取链接失败]"
			}
			state := "有效"
			if _, statErr := os.Stat(fullPath); statErr != nil {
				state = "悬空(目标不存在)"
			}
			if appName != "" && !strings.HasPrefix(entry.Name(), appName) {
				continue
			}
			fmt.Printf("  [链接] %-60s -> %s  %s\n", entry.Name(), target, state)
		case info.IsDir():
			continue
		default:
			files++
			if appName != "" && !strings.HasPrefix(entry.Name(), appName) {
				continue
			}
			name, start, stopped := parseSessionName(entry.Name())
			kind := "未知命名"
			if name != "" && !start.IsZero() {
				if stopped {
					kind = "归档(已停止)"
				} else {
					kind = "会话(未归档)" + warnIfOld(start)
				}
			}
			fmt.Printf("  [文件] %-60s %8d 字节  %s  %s\n",
				entry.Name(), info.Size(), kind, info.ModTime().Format("2006-01-02 15:04:05"))
		}
	}

	fmt.Printf("\n汇总: 普通文件 %d 个，软链接 %d 个\n", files, links)

	if links > 0 {
		fmt.Println("\n提示: 归档文件与会话日志都应是普通文件；出现软链接说明是旧版本遗留的脏数据，")
		fmt.Println("      执行一次 start 会自动清理本应用的会话命名脏链接。")
	}

	// 软链接语义自检：确认本机 os.Symlink/os.Rename 行为符合预期
	fmt.Println()
	return checkSymlinkSemantics(logsDir)
}

// warnIfOld 未归档会话额外提示（可能是异常退出留下的）
func warnIfOld(start time.Time) string {
	if time.Since(start) > 24*time.Hour {
		return "  ← 超过 24h 仍未归档，可能是异常退出遗留"
	}
	return ""
}

// checkSymlinkSemantics 在当前机器上实测软链接 + rename 语义
func checkSymlinkSemantics(logsDir string) error {
	probe := filepath.Join(logsDir, ".symlink-probe.log")
	target := filepath.Join(logsDir, ".symlink-probe-target.log")

	cleanup := func() {
		_ = os.Remove(probe)
		_ = os.Remove(target)
		_ = os.Remove(probe + ".moved")
	}
	cleanup()
	defer cleanup()

	if err := os.WriteFile(target, []byte("probe\n"), 0644); err != nil {
		return fmt.Errorf("软链接自检: 创建探测文件失败: %w", err)
	}
	if err := os.Symlink(filepath.Base(target), probe); err != nil {
		fmt.Printf("软链接自检: 跳过（当前环境不支持软链接: %v）\n", err)
		fmt.Println("            Windows 上 <app>.log 会退化为“按最新会话日志查找”，Linux 上正常工作。")
		return nil
	}

	moved := probe + ".moved"
	if err := os.Rename(probe, moved); err != nil {
		return fmt.Errorf("软链接自检: rename 失败: %w", err)
	}

	movedInfo, err := os.Lstat(moved)
	if err != nil {
		return fmt.Errorf("软链接自检: 读取 rename 结果失败: %w", err)
	}

	if movedInfo.Mode()&os.ModeSymlink != 0 {
		fmt.Println("软链接自检: 通过（rename 作用于链接本身 → 代码已改为先解析真实路径再归档）")
	} else {
		fmt.Println("软链接自检: 当前系统 rename 作用于目标文件（与设计假设不同，请反馈）")
	}

	if _, err := os.Stat(target); err != nil {
		fmt.Println("           注意: 探测目标文件被移动，归档逻辑需复核")
	}

	return nil
}
