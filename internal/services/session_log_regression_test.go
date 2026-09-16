package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/logger"
	"app-manager-go-v1/internal/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupLogDir 准备一个隔离的 project_backend/logs 目录
func setupLogDir(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	logsDir := filepath.Join(projectRoot, "project_backend", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	config.Init()
	logger.Init()
	config.AppConfig.Global.ProjectRoot = projectRoot

	return logsDir
}

// symlinkTargets 返回目录内所有软链接及其（相对）目标
func symlinkTargets(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	links := make(map[string]string)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		links[entry.Name()] = target
	}
	return links
}

// TestStopTwiceDoesNotChainSymlinks 复现线上问题：
// 同一个运行周期内被 stop 两次（daemon 优雅退出 + stop 命令）时，
// 不允许再把 <app>.log 软链接本身改名成 -停止 文件，也不允许出现 -停止(2).log
func TestStopTwiceDoesNotChainSymlinks(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-people-organization-backend"

	// 第一次运行: 启动 -> 写日志 -> ln app.log
	session := NewSessionLogPath(app, time.Now().Add(-2*time.Minute))
	if err := os.WriteFile(session, []byte("run 1 output\n"), 0644); err != nil {
		t.Fatal(err)
	}
	linkCurrentLog(app, session)

	svc := NewApplicationService()

	// 第一次停止: 归档 + 清理链接
	if _, err := svc.archiveSessionLog(&models.Application{Name: app, LogFile: session}); err != nil {
		t.Fatalf("第一次归档失败: %v", err)
	}

	// 第二次停止: 不应再次归档，更不应把软链接改名
	app2 := &models.Application{Name: app, LogFile: session}
	if _, err := svc.archiveSessionLog(app2); err != nil {
		t.Fatalf("第二次归档失败: %v", err)
	}

	// === 断言: 目录里不能有任何软链接（停止后链接必须清掉） ===
	if links := symlinkTargets(t, logsDir); len(links) != 0 {
		t.Fatalf("停止后不应残留软链接: %v", links)
	}

	// === 断言: 不能出现 停止(2) / 停止(3) 这种冲突副本 ===
	entries, _ := os.ReadDir(logsDir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "停止(") {
			t.Fatalf("出现冲突副本文件: %s", entry.Name())
		}
	}

	// === 断言: 只应有一个归档文件，内容完好 ===
	sessions := ListLogSessions(app)
	if len(sessions) != 1 {
		t.Fatalf("会话数量应为 1，实际 %d: %v", len(sessions), sessionNames(sessions))
	}
	if !sessions[0].Stopped {
		t.Fatalf("会话应处于已归档状态: %s", filepath.Base(sessions[0].Path))
	}
	if data, err := os.ReadFile(sessions[0].Path); err != nil || string(data) != "run 1 output\n" {
		t.Fatalf("归档内容异常: %q %v", data, err)
	}
}

// TestStartStopRestartCyclesStayClean 连续 启动/停止/重启 多轮后目录必须保持干净
func TestStartStopRestartCyclesStayClean(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"
	svc := NewApplicationService()

	var lastLog string
	for i := 1; i <= 3; i++ {
		// 启动: 准备会话日志（使用递增的启动时间，避免同秒重名）
		cfg := config.AppConfig
		_ = cfg
		sessionStart := time.Now().Add(time.Duration(i) * time.Second)
		session := NewSessionLogPath(app, sessionStart)
		if err := os.WriteFile(session, []byte("run output\n"), 0644); err != nil {
			t.Fatal(err)
		}
		linkCurrentLog(app, session)
		lastLog = session

		// 运行中: 当前日志必须是这次会话
		if cur := ResolveCurrentLog(app); filepath.Base(cur) != filepath.Base(session) {
			t.Fatalf("第 %d 轮当前日志错误: %s", i, cur)
		}

		// 停止: 归档（模拟 stop 与 daemon 退出各调用一次）
		appModel := &models.Application{Name: app, LogFile: session}
		if _, err := svc.archiveSessionLog(appModel); err != nil {
			t.Fatalf("第 %d 轮归档失败: %v", i, err)
		}
		if _, err := svc.archiveSessionLog(appModel); err != nil {
			t.Fatalf("第 %d 轮重复归档失败: %v", i, err)
		}
	}

	// 停止后无软链接
	if links := symlinkTargets(t, logsDir); len(links) != 0 {
		t.Fatalf("停止后不应残留软链接: %v", links)
	}

	// 三轮运行 => 三个归档文件，全部有 -停止 标记且无冲突副本
	sessions := ListLogSessions(app)
	if len(sessions) != 3 {
		t.Fatalf("应有 3 个归档会话，实际 %d: %v", len(sessions), sessionNames(sessions))
	}
	for _, session := range sessions {
		if !session.Stopped {
			t.Fatalf("归档会话缺少 -停止 标记: %s", filepath.Base(session.Path))
		}
		if strings.Contains(filepath.Base(session.Path), "停止(") {
			t.Fatalf("出现冲突副本: %s", filepath.Base(session.Path))
		}
		data, err := os.ReadFile(session.Path)
		if err != nil || string(data) != "run output\n" {
			t.Fatalf("会话内容异常: %s %q %v", session.Path, data, err)
		}
	}
	_ = lastLog
}

// TestMessyLegacyLinksSelfHeal 线上已产生的历史脏数据应被自动清理
// 构造线上现状: 归档文件 + <app>.log 指向归档文件 + 会话命名的脏软链接
func TestMessyLegacyLinksSelfHeal(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-people-organization-backend"

	// 构造线上现状: 一个归档文件 + 指向它的 <app>.log
	archived := filepath.Join(logsDir, app+"-2026-09-16-154954-停止.log")
	if err := os.WriteFile(archived, []byte("old run\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(archived), LegacyLogPath(app)); err != nil {
		t.Skipf("当前环境不支持软链接（需在 Linux 上验证）: %v", err)
	}

	// 新版本启动时应清理脏链接并建立本次会话链接
	svc := NewApplicationService()
	got, err := svc.prepareSessionLog(&models.Application{Name: app})
	if err != nil {
		t.Fatalf("准备会话日志失败: %v", err)
	}

	links := symlinkTargets(t, logsDir)
	if len(links) != 1 {
		t.Fatalf("清理后应只剩 <app>.log 一个软链接，实际: %v", links)
	}
	if _, ok := links[app+".log"]; !ok {
		t.Fatalf("缺少 <app>.log 软链接: %v", links)
	}
	if target := links[app+".log"]; target != filepath.Base(got) {
		t.Fatalf("<app>.log 应指向本次会话 %s，实际 %s", filepath.Base(got), target)
	}

	// 历史归档文件必须保留
	if data, err := os.ReadFile(archived); err != nil || string(data) != "old run\n" {
		t.Fatalf("历史归档被破坏: %q %v", data, err)
	}
	if n := CountLogSessions(app); n != 1 {
		t.Fatalf("历史归档会话数错误: %d", n)
	}
}

// TestCleanupStraySessionLinks 历史脏链接清理只针对本应用的会话/归档命名链接
func TestCleanupStraySessionLinks(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"
	other := "other-app"

	// 造几个真实归档文件供链接指向
	var archives []string
	for i := 1; i <= 3; i++ {
		path := filepath.Join(logsDir, freshName(app, i))
		if err := os.WriteFile(path, []byte("archived\n"), 0644); err != nil {
			t.Fatal(err)
		}
		archives = append(archives, path)
	}
	otherPath := filepath.Join(logsDir, freshName(other, 1))
	if err := os.WriteFile(otherPath, []byte("other\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 脏链接: 会话/归档命名的软链接（本应用 + 其他应用）
	dirty := []struct{ link, target string }{
		{app + "-2026-09-16-154954-停止(2).log", filepath.Base(archives[0])},
		{app + "-2026-09-16-155151-停止.log", filepath.Base(archives[1])},
		{app + "-2026-09-16-160000.log", filepath.Base(archives[2])},
		{other + "-2026-09-16-160000.log", filepath.Base(otherPath)},
	}
	created := 0
	for _, item := range dirty {
		if err := os.Symlink(item.target, filepath.Join(logsDir, item.link)); err != nil {
			t.Skipf("当前环境不支持软链接（需在 Linux 上验证）: %v", err)
		}
		created++
	}

	// <app>.log 运行期链接不应被清理
	if err := os.Symlink(filepath.Base(archives[2]), LegacyLogPath(app)); err != nil {
		t.Skipf("当前环境不支持软链接: %v", err)
	}

	removed := cleanupStraySessionLinks(app)
	if removed != 3 {
		t.Fatalf("应清理 3 个本应用的脏链接，实际 %d", removed)
	}

	links := symlinkTargets(t, logsDir)
	if len(links) != 2 {
		t.Fatalf("应保留 <app>.log 与其他应用的链接，实际: %v", links)
	}
	if _, ok := links[app+".log"]; !ok {
		t.Fatalf("<app>.log 被误删: %v", links)
	}
	if _, ok := links[other+"-2026-09-16-160000.log"]; !ok {
		t.Fatalf("其他应用的链接被误删: %v", links)
	}

	// 真实归档文件一个都不能少
	for _, archive := range append(archives, otherPath) {
		if _, err := os.Stat(archive); err != nil {
			t.Fatalf("真实归档文件丢失: %s %v", archive, err)
		}
	}
	_ = created
}

// TestResolveRealLogPath 软链接解析：软链接必须解析到目标，悬空链接视为无日志
func TestResolveRealLogPath(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"

	// 普通文件 -> 原样返回
	session := filepath.Join(logsDir, freshName(app, 1))
	if err := os.WriteFile(session, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := resolveRealLogPath(session); got != session {
		t.Fatalf("普通文件应原样返回: %s", got)
	}

	// 不存在的路径 -> 空
	if got := resolveRealLogPath(filepath.Join(logsDir, "nope.log")); got != "" {
		t.Fatalf("不存在的路径应返回空: %s", got)
	}

	// 悬空软链接 -> 空（避免把链接当日志归档）
	dangling := filepath.Join(logsDir, app+".log")
	if err := os.Symlink("not-exist.log", dangling); err == nil {
		if got := resolveRealLogPath(dangling); got != "" {
			t.Fatalf("悬空链接应返回空: %s", got)
		}
		if _, err := archiveLogFile(dangling); err == nil {
			t.Fatal("悬空软链接不应被直接归档")
		}
	}

	// 软链接 -> 解析到真实文件（Windows 无权限时跳过）
	target := filepath.Join(logsDir, freshName(app, 2))
	if err := os.WriteFile(target, []byte("y\n"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(logsDir, app+"-link.log")
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Skipf("当前环境不支持软链接（需在 Linux 上验证）: %v", err)
	}
	got := resolveRealLogPath(link)
	if filepath.Base(got) != filepath.Base(target) {
		t.Fatalf("软链接应解析到目标 %s，实际 %s", filepath.Base(target), got)
	}

	// 关键回归: 对软链接归档时必须作用于目标文件，链接本身被清理
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("测试前置条件失败: 期望软链接")
	}
	archivedTarget, err := archiveLogFile(got)
	if err != nil {
		t.Fatalf("归档真实文件失败: %v", err)
	}
	if filepath.Base(archivedTarget) != filepath.Base(target) {
		t.Fatalf("归档应改名真实文件 %s，实际 %s", filepath.Base(target), filepath.Base(archivedTarget))
	}
	if !hasStoppedMark(filepath.Base(archivedTarget)) {
		t.Fatalf("归档命名缺少 -停止: %s", filepath.Base(archivedTarget))
	}
	// 软链接原样存在但已悬空 —— 运行期链接由 archiveSessionLog 负责清理
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("软链接不应被归档操作移动: %v", err)
	}
}

// TestHardLinkRenameSemantics 复现事故内核：
// os.Rename 走的是"文件名 -> inode"这一层，改名软链接只会移动链接本身、不改名目标文件。
// 这里用硬链接（与软链接同一层语义，且 Windows 普通用户即可创建）证明：
// 归档必须作用于真实文件，否则原文件会一直被写入、归档名只是链接或压根没有日志。
func TestHardLinkRenameSemantics(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"

	realFile := filepath.Join(logsDir, app+"-2026-09-16-155015.log")
	if err := os.WriteFile(realFile, []byte("live output\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 固定 mtime，避免测试运行时刻混进归档文件名
	writeTime := time.Date(2026, 9, 16, 15, 50, 15, 0, time.Local)
	if err := os.Chtimes(realFile, writeTime, writeTime); err != nil {
		t.Fatal(err)
	}

	// <app>.log 以硬链接形式"指向"真实日志（等价于软链接在 rename 语义下的表现）
	linkPath := LegacyLogPath(app)
	if err := os.Link(realFile, linkPath); err != nil {
		t.Skipf("当前环境不支持硬链接: %v", err)
	}

	// 归档 <app>.log：必须改名真实文件，而不是把"链接名"当成日志
	archived, err := archiveLogFile(resolveRealLogPath(linkPath))
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	if filepath.Base(archived) == app+".log" {
		t.Fatalf("归档路径不应是 <app>.log: %s", archived)
	}
	if !hasStoppedMark(filepath.Base(archived)) {
		t.Fatalf("归档命名缺少 -停止: %s", filepath.Base(archived))
	}

	// 内容必须跟着真实文件走到归档文件里
	if data, err := os.ReadFile(archived); err != nil || string(data) != "live output\n" {
		t.Fatalf("归档内容异常: %q %v", data, err)
	}

	// 归档后 <app>.log 这个"链接名"应被清理（运行期链接只在使用期间存在）
	removeCurrentLogLink(app)
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("<app>.log 链接应被清理: %v", err)
	}

	// 归档名必须基于被归档的真实文件（而不是 <app>.log 这个链接名）
	if got := filepath.Base(archived); got != app+"-2026-09-16-155015-停止.log" {
		t.Fatalf("归档名应保持原文件的启动时间戳，实际: %s", got)
	}

	// 硬链接语义下原文件仍有另一个目录项，会由 prepareSessionLog 在下次启动时补归档
	svc := NewApplicationService()
	if _, err := svc.prepareSessionLog(&models.Application{Name: app}); err != nil {
		t.Fatalf("准备下次会话失败: %v", err)
	}
	if _, err := os.Lstat(realFile); !os.IsNotExist(err) {
		t.Fatalf("原会话文件应在下次启动时被归档: %v", err)
	}
	for _, session := range ListLogSessions(app) {
		if !session.Stopped {
			t.Fatalf("启动后不应残留未归档会话: %s", filepath.Base(session.Path))
		}
	}
}

// freshName 生成互不相同且符合会话命名的归档文件名
func freshName(appName string, index int) string {
	return appName + "-2026-09-16-15515" + string(rune('0'+index)) + "-停止.log"
}

func sessionNames(sessions []*LogSession) []string {
	names := make([]string, 0, len(sessions))
	for _, session := range sessions {
		names = append(names, filepath.Base(session.Path))
	}
	return names
}
