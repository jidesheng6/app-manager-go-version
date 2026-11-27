package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
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
//	本次运行   : <app>-<启动时间>启动.log                    例如 party-auth-2025-01-01-101530启动.log
//	已归档会话 : <app>-<启动时间>启动-<停止时间>停止.log      例如 party-auth-2025-01-01-101530启动-2025-01-01-183000停止.log
//	当前快捷   : <app>.log （软链接，仅运行期存在，指向本次会话日志）
//
// 兼容旧命名（上一版产生，仍可识别、可继续归档）：
//
//	<app>-<启动时间>.log
//	<app>-<启动时间>-停止.log
//
// 说明：日志按“运行周期”分段，每次 start 新建一个文件，stop/restart 时把当前文件改名归档，
// 文件名同时记录启动与停止时刻，历史上任何一次启动的输出都不会被覆盖，
// 程序被 kill -9 时也会在下次启动前补归档。

const (
	// SessionTimeLayout 会话日志文件名中的时间格式: yyyy-MM-dd-HHmmss
	SessionTimeLayout = "2006-01-02-150405"

	// sessionTimePattern 时间片段的形状（4-2-2-6 位数字），用于拼接正则
	sessionTimePattern = `\d{4}-\d{2}-\d{2}-\d{6}`

	// startMark 启动标记，跟在启动时间之后
	startMark = "启动"
	// stopMark 停止标记，跟在停止时间之后
	stopMark = "停止"

	// legacyStoppedSuffix 旧版归档标记 <app>-<启动时间>-停止.log
	legacyStoppedSuffix = "-停止"

	// logExt 会话日志扩展名（归档保持明文，便于崩溃排障直接阅读）
	logExt = ".log"
	// currentPointerSuffix 软链接不可用时的指针文件后缀（Windows 常见）
	// 内容为一行：会话日志文件名（basename）
	currentPointerSuffix = ".log.current"
)

var (
	// sessionLogRegex 匹配旧版会话命名 <app>-<启动时间>.log
	sessionLogRegex = regexp.MustCompile(`^(.*?)-(` + sessionTimePattern + `)\.log$`)
	// legacyStoppedLogRegex 匹配旧版归档命名 <app>-<启动时间>-停止.log
	legacyStoppedLogRegex = regexp.MustCompile(`^(.*?)-(` + sessionTimePattern + `)-停止\.log$`)
	// legacyCollisionLogRegex 匹配旧版同秒冲突命名 <app>-<启动时间>-停止(2).log
	legacyCollisionLogRegex = regexp.MustCompile(`^(.*)-(` + sessionTimePattern + `)-停止\(\d+\)\.log$`)
	// archivedLogRegex 匹配归档命名 <app>-<启动时间>启动-<停止时间>停止.log
	archivedLogRegex = regexp.MustCompile(`^(.*)-(` + sessionTimePattern + `)启动-(` + sessionTimePattern + `)停止\.log$`)
	// startedLogRegex 匹配会话命名 <app>-<启动时间>启动.log
	startedLogRegex = regexp.MustCompile(`^(.*)-(` + sessionTimePattern + `)启动\.log$`)
	// stopTailRegex 从完整路径中提取末尾的 -<停止时间>停止.log
	stopTailRegex = regexp.MustCompile(`-(` + sessionTimePattern + `)停止\.log$`)
	// collisionSuffixRegex 匹配旧版同秒冲突产生的 (2)/(3) 后缀
	collisionSuffixRegex = regexp.MustCompile(`^\(\d+\)$`)
)

// LogSession 表示一次运行周期产生的日志文件
type LogSession struct {
	AppName   string    // 应用名称
	Path      string    // 日志文件完整路径
	StartTime time.Time // 会话启动时间（文件名中的启动时间）
	StopTime  time.Time // 停止时间（已归档的文件才有）
	Stopped   bool      // 是否已归档
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
	return filepath.Join(config.GetLogsDir(), appName+"-"+start.Format(SessionTimeLayout)+startMark+logExt)
}

// hasStoppedMark 判断文件名是否带归档标记
func hasStoppedMark(fileName string) bool {
	baseName := filepath.Base(fileName)
	base := strings.TrimSuffix(baseName, logExt)
	return stopTailRegex.MatchString(baseName) || strings.HasSuffix(base, legacyStoppedSuffix)
}

// sessionAppName 从会话日志文件名中还原应用名称
func sessionAppName(fileName string) string {
	base := filepath.Base(fileName)

	// 新版会话/归档命名: 去掉 启动/停止 时间片段
	if m := archivedLogRegex.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	if m := startedLogRegex.FindStringSubmatch(base); m != nil {
		return m[1]
	}

	// 旧版命名: 去掉 -<时间> 与 -停止 后缀
	if m := sessionLogRegex.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	if m := legacyStoppedLogRegex.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	// 旧版同秒冲突命名 <app>-<时间>-停止(2).log
	if m := legacyCollisionLogRegex.FindStringSubmatch(base); m != nil {
		return m[1]
	}

	// 其他命名（例如旧版 <app>.log）: 只去扩展名
	return strings.TrimSuffix(base, logExt)
}

// sessionStartSegment 从会话日志文件名中取出启动时间片段
func sessionStartSegment(fileName string) string {
	base := filepath.Base(fileName)

	if m := archivedLogRegex.FindStringSubmatch(base); m != nil {
		return m[2]
	}
	if m := startedLogRegex.FindStringSubmatch(base); m != nil {
		return m[2]
	}
	if m := sessionLogRegex.FindStringSubmatch(base); m != nil {
		return m[2]
	}
	if m := legacyStoppedLogRegex.FindStringSubmatch(base); m != nil {
		return m[2]
	}
	if m := legacyCollisionLogRegex.FindStringSubmatch(base); m != nil {
		return m[2]
	}
	return ""
}

// parseSessionTime 解析文件名中的时间片段，失败时返回零值
func parseSessionTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.ParseInLocation(SessionTimeLayout, value, time.Local)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// archivedSessionPath 生成归档后的日志路径: <app>-<启动时间>启动-<停止时间>停止.log
// filePath 为当前会话日志；fallbackTime 用于无法从文件名解析启动时间时的时间戳
func archivedSessionPath(filePath string, stopTime time.Time) string {
	appName := sessionAppName(filePath)

	start := parseSessionTime(sessionStartSegment(filepath.Base(filePath)))
	if start.IsZero() {
		start = stopTime
	}
	if start.IsZero() {
		start = time.Now()
	}
	if stopTime.IsZero() {
		stopTime = time.Now()
	}

	// 同秒停止时把停止时间顺延 1 秒，避免启动与停止时间戳完全相同造成歧义
	if stopTime.Equal(start) {
		stopTime = stopTime.Add(time.Second)
	}

	return filepath.Join(filepath.Dir(filePath),
		appName+"-"+start.Format(SessionTimeLayout)+startMark+"-"+stopTime.Format(SessionTimeLayout)+stopMark+logExt)
}

// parseSessionName 解析会话日志文件名（新旧命名都支持）
// 非会话日志命名（例如旧版 <app>.log）返回 zero=true
func parseSessionName(fileName string) (session LogSession, zero bool) {
	base := filepath.Base(fileName)

	// 新版归档: <app>-<启动>启动-<停止>停止.log
	if m := archivedLogRegex.FindStringSubmatch(base); m != nil {
		return LogSession{
			AppName:   m[1],
			StartTime: parseSessionTime(m[2]),
			StopTime:  parseSessionTime(m[3]),
			Stopped:   true,
		}, false
	}

	// 新版会话: <app>-<启动>启动.log
	if m := startedLogRegex.FindStringSubmatch(base); m != nil {
		return LogSession{AppName: m[1], StartTime: parseSessionTime(m[2])}, false
	}

	// 旧版归档: <app>-<启动>-停止.log
	if m := legacyStoppedLogRegex.FindStringSubmatch(base); m != nil {
		return LogSession{AppName: m[1], StartTime: parseSessionTime(m[2]), Stopped: true}, false
	}

	// 旧版同秒冲突归档: <app>-<启动>-停止(2).log
	if m := legacyCollisionLogRegex.FindStringSubmatch(base); m != nil {
		return LogSession{AppName: m[1], StartTime: parseSessionTime(m[2]), Stopped: true}, false
	}

	// 旧版会话: <app>-<启动>.log
	if m := sessionLogRegex.FindStringSubmatch(base); m != nil {
		return LogSession{AppName: m[1], StartTime: parseSessionTime(m[2])}, false
	}

	return LogSession{}, true
}

// sessionIsForApp 判断会话日志是否属于指定应用
// 容忍旧版同秒冲突产生的 (2)/(3) 后缀，保证脏数据也能被清理和归档
func sessionIsForApp(session LogSession, appName string) bool {
	if session.AppName == appName {
		return true
	}

	// 形如 <app>(2)：比对去掉冲突后缀后的名字
	if idx := strings.LastIndex(session.AppName, "("); idx > 0 {
		base := session.AppName[:idx]
		if collisionSuffixRegex.MatchString(session.AppName[idx:]) && base == appName {
			return true
		}
	}

	return false
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
		if session, zero := parseSessionName(name); zero || !sessionIsForApp(session, appName) {
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

// archiveLogFile 把正在写入的日志文件改名归档为
// <app>-<启动时间>启动-<停止时间>停止.log
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

	target := nextArchivedPath(archivedSessionPath(path, info.ModTime()))
	if filepath.Clean(target) == filepath.Clean(path) {
		return path, nil
	}

	if err := os.Rename(path, target); err != nil {
		return "", err
	}

	// 链接是绝对路径时直接删；相对路径时按链接文本判断是否指向刚被改名的文件
	removeLinkTo(appNameOfSessionPath(path), filepath.Base(path))
	clearCurrentPointer(appNameOfSessionPath(path))

	return target, nil
}

// logsDirReady 配置已初始化时才读写指针文件（单测里可能未 Init）
func logsDirReady() bool {
	return config.AppConfig != nil
}

// currentPointerPath 返回软链接失败时使用的指针文件路径
func currentPointerPath(appName string) string {
	return filepath.Join(config.GetLogsDir(), appName+currentPointerSuffix)
}

// writeCurrentPointer 写入当前会话指针（一行 basename）
func writeCurrentPointer(appName, target string) {
	if appName == "" || target == "" || !logsDirReady() {
		return
	}
	_ = os.WriteFile(currentPointerPath(appName), []byte(filepath.Base(target)+"\n"), 0644)
}

// readCurrentPointer 读取指针指向的真实日志路径；无效时返回空
func readCurrentPointer(appName string) string {
	if appName == "" || !logsDirReady() {
		return ""
	}
	data, err := os.ReadFile(currentPointerPath(appName))
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(data))
	if name == "" || strings.Contains(name, string(os.PathSeparator)) || strings.Contains(name, "/") {
		return ""
	}
	full := filepath.Join(config.GetLogsDir(), name)
	if _, err := os.Stat(full); err != nil {
		return ""
	}
	return full
}

// clearCurrentPointer 删除指针文件
func clearCurrentPointer(appName string) {
	if appName == "" || !logsDirReady() {
		return
	}
	_ = os.Remove(currentPointerPath(appName))
}

// appNameOfSessionPath 从会话日志路径还原应用名称
func appNameOfSessionPath(path string) string {
	return sessionAppName(filepath.Base(path))
}

// removeLinkTo 删除指向 oldName 的 <app>.log 软链接（归档改名后调用，避免链接悬空）
func removeLinkTo(appName, oldName string) {
	if appName == "" || !logsDirReady() {
		return
	}

	linkPath := CurrentLogLinkPath(appName)
	info, err := os.Lstat(linkPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		return
	}

	// 链接文本可能是相对名或绝对路径
	if filepath.Base(target) == oldName {
		_ = os.Remove(linkPath)
	}
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
//  2. <app>.log.current 指针文件（软链接不可用时的回退，如 Windows）
//  3. 最新一次未归档的会话日志
//  4. 最近一次归档日志（应用已停止时，让它指向最后一次运行的输出）
//
// 找不到时返回空字符串
func ResolveCurrentLog(appName string) string {
	linkPath := CurrentLogLinkPath(appName)

	// 1) 有效软链接（且未指向已归档文件）→ 返回目标真实路径
	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if _, err := os.Stat(linkPath); err == nil {
			if real, err := os.Readlink(linkPath); err == nil {
				if hasStoppedMark(filepath.Base(real)) {
					_ = os.Remove(linkPath) // 过期链接
				} else if filepath.IsAbs(real) {
					return real
				} else {
					return filepath.Join(filepath.Dir(linkPath), real)
				}
			}
		}
	}

	// 2) 指针文件（软链接不可用时的回退）
	if ptr := readCurrentPointer(appName); ptr != "" {
		return ptr
	}

	// 3) 最新未归档会话（比遗留的普通 <app>.log 更可信）
	if newest := newestActiveSessionLog(appName); newest != nil {
		return newest.Path
	}

	// 4) 遗留的真实 <app>.log（旧版固定路径）
	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink == 0 && !info.IsDir() {
		return linkPath
	}

	// 5) 最近一次归档
	if newest := newestSessionLog(appName); newest != nil {
		return newest.Path
	}

	return ""
}

// linkCurrentLog 建立运行期软链接 <app>.log -> 本次会话日志
// 拒绝指向归档文件：软链接只在应用运行期间有意义，停止后由 removeCurrentLogLink 清理。
// 软链接失败时写入 <app>.log.current 指针，保证 Windows 等环境仍能解析“当前日志”。
func linkCurrentLog(appName, target string) {
	if target == "" || hasStoppedMark(filepath.Base(target)) {
		return
	}

	linkPath := CurrentLogLinkPath(appName)
	if filepath.Clean(linkPath) == filepath.Clean(target) {
		return
	}

	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink == 0 {
		// 旧命名真实文件仍在，不能覆盖；仍写指针以便 Resolve 能找到新会话
		writeCurrentPointer(appName, target)
		return
	}

	_ = os.Remove(linkPath)
	if err := os.Symlink(filepath.Base(target), linkPath); err != nil {
		writeCurrentPointer(appName, target)
		return
	}
	clearCurrentPointer(appName) // 软链接成功则不需要指针
}

// removeCurrentLogLink 删除旧的“当前日志”软链接与指针文件
func removeCurrentLogLink(appName string) {
	linkPath := CurrentLogLinkPath(appName)
	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(linkPath)
	}
	clearCurrentPointer(appName)
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
		name := entry.Name()
		// 跳过指针文件与软链接探测残留
		if strings.HasSuffix(name, currentPointerSuffix) {
			continue
		}

		session, zero := parseSessionName(name)
		if zero || session.StartTime.IsZero() || !sessionIsForApp(session, appName) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		session.AppName = appName
		session.Path = filepath.Join(logsDir, entry.Name())
		session.Size = info.Size()
		session.ModTime = info.ModTime()

		sessions = append(sessions, &session)
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

// describeKeepPolicy 生成保留策略的可读描述
func describeKeepPolicy(keepCount int) string {
	switch {
	case keepCount < 0:
		return "不限制，保留全部留痕"
	case keepCount == 0:
		return "不留归档，每次停止后清理（只保留当前正在写入的日志）"
	default:
		return fmt.Sprintf("每个应用保留最近 %d 份归档日志，超出后自动删除最旧的", keepCount)
	}
}

// excessArchivedLogs 返回超出保留份数的归档日志数量（负数表示不限制）
func excessArchivedLogs(sessions []*LogSession, keepCount int) int {
	if keepCount < 0 {
		return 0
	}

	archived := 0
	for _, session := range sessions {
		if session.Stopped {
			archived++
		}
	}

	if archived <= keepCount {
		return 0
	}
	return archived - keepCount
}

// pruneArchivedLogs 按保留份数清理最旧的归档日志
//
// 保留策略（份数由配置 application.log_keep_count 决定，默认 7）：
//   - 正整数: 保留最近 N 份归档，其余删除
//   - 0     : 归档全部删除（只留当前正在写入的日志）
//   - 负数  : 不限制，全部保留
//
// 只处理**已归档**的日志，正在写入的会话日志永远不动。
//
// 返回本次删除的份数
func pruneArchivedLogs(appName string, keepCount int) int {
	if keepCount < 0 {
		return 0 // 负数 = 不限制
	}

	var archived []*LogSession
	for _, session := range ListLogSessions(appName) {
		if session.Stopped {
			archived = append(archived, session)
		}
	}

	if len(archived) <= keepCount {
		return 0
	}

	removed := 0
	for _, session := range archived[keepCount:] {
		if err := os.Remove(session.Path); err != nil {
			logger.Warning("清理旧日志失败 %s: %v", filepath.Base(session.Path), err)
			continue
		}
		logger.Info("已清理超出保留份数的旧日志: %s（保留最近 %d 份）",
			filepath.Base(session.Path), keepCount)
		removed++
	}

	return removed
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
			session, zero := parseSessionName(entry.Name())
			kind := "未知命名"
			if !zero && !session.StartTime.IsZero() {
				if session.Stopped {
					kind = "归档(已停止)"
					if !session.StopTime.IsZero() {
						kind += " 停止于 " + session.StopTime.Format("15:04:05")
					}
				} else {
					kind = "会话(未归档)" + warnIfOld(session.StartTime)
				}
			}
			fmt.Printf("  [文件] %-60s %8d 字节  %s  %s\n",
				entry.Name(), info.Size(), kind, info.ModTime().Format("2006-01-02 15:04:05"))
		}
	}

	fmt.Printf("\n汇总: 普通文件 %d 个，软链接 %d 个\n", files, links)
	fmt.Printf("保留策略: %s（application.log_keep_count）\n", describeKeepPolicy(config.KeepLogCount()))

	if links > 0 {
		fmt.Println("\n提示: 归档文件与会话日志都应是普通文件；出现软链接说明是旧版本遗留的脏数据，")
		fmt.Println("      执行一次 start 会自动清理本应用的会话命名脏链接。")
	}

	// 超出保留份数的归档日志（下次 start/stop 会自动清理）
	if appName != "" {
		if sessions := ListLogSessions(appName); len(sessions) > 0 {
			if excess := excessArchivedLogs(sessions, config.KeepLogCount()); excess > 0 {
				fmt.Printf("\n超量日志: %d 份将在下次 start/stop 时自动清理（最旧的先删）\n", excess)
			}
		}
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
