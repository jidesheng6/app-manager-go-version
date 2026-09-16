package services

import (
	"app-manager-go-v1/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPruneArchivedLogsKeepsConfiguredCount 每个应用只保留配置的份数
func TestPruneArchivedLogsKeepsConfiguredCount(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"
	svc := NewApplicationService()

	config.AppConfig.Application.LogKeepCount = 3
	if got := config.KeepLogCount(); got != 3 {
		t.Fatalf("保留份数读取错误: %d", got)
	}

	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.Local)

	// 跑 6 轮 启动 -> 停止
	for i := 0; i < 6; i++ {
		start := base.Add(time.Duration(i) * time.Minute)
		session := NewSessionLogPath(app, start)
		if err := writeFile(t, session, fmt.Sprintf("run %d\n", i+1), start.Add(30*time.Second)); err != nil {
			t.Fatal(err)
		}
		linkCurrentLog(app, session)

		if _, err := svc.archiveSessionLog(newApp(app, session)); err != nil {
			t.Fatalf("第 %d 轮归档失败: %v", i+1, err)
		}
		if removed := pruneArchivedLogs(app, config.KeepLogCount()); removed > 0 {
			t.Logf("第 %d 轮清理了 %d 份旧日志", i+1, removed)
		}
	}

	entries, err := readDirNames(logsDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("目录内容: %v", entries)

	// 只应保留最近 3 份归档
	sessions := ListLogSessions(app)
	if len(sessions) != 3 {
		t.Fatalf("应只保留 3 份，实际 %d: %v", len(sessions), sessionNames(sessions))
	}
	for _, session := range sessions {
		if !session.Stopped {
			t.Fatalf("保留的应是归档日志: %s", filepath.Base(session.Path))
		}
	}

	// 保留的必须是最近的 3 轮（第 3、4、5、6 次里的后 3 次 => 第 4/5/6 次）
	wantKept := map[string]bool{}
	for _, i := range []int{3, 4, 5} { // 0-based: 第 4、5、6 轮
		wantKept[fmt.Sprintf("party-auth-%s启动-%s停止.log",
			base.Add(time.Duration(i)*time.Minute).Format(SessionTimeLayout),
			base.Add(time.Duration(i)*time.Minute+30*time.Second).Format(SessionTimeLayout))] = true
	}
	for _, session := range sessions {
		if !wantKept[filepath.Base(session.Path)] {
			t.Fatalf("保留了不该保留的日志: %s（期望 %v）", filepath.Base(session.Path), wantKept)
		}
	}

	// 每个被删除的轮次都不应残留
	for _, i := range []int{0, 1, 2} {
		gone := fmt.Sprintf("party-auth-%s启动-%s停止.log",
			base.Add(time.Duration(i)*time.Minute).Format(SessionTimeLayout),
			base.Add(time.Duration(i)*time.Minute+30*time.Second).Format(SessionTimeLayout))
		if _, err := os.Lstat(filepath.Join(logsDir, gone)); !os.IsNotExist(err) {
			t.Fatalf("最旧的日志应被清理: %s", gone)
		}
	}
}

// TestPruneNeverDeletesActiveSession 正在写入的会话日志永远不被清理
func TestPruneNeverDeletesActiveSession(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"
	config.AppConfig.Application.LogKeepCount = 1

	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.Local)

	// 5 份归档
	for i := 0; i < 5; i++ {
		start := base.Add(time.Duration(i) * time.Minute)
		path := NewSessionLogPath(app, start)
		if err := writeFile(t, path, "archived\n", start.Add(10*time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := archiveLogFile(path); err != nil {
			t.Fatal(err)
		}
	}

	// 1 份正在写入的会话
	active := NewSessionLogPath(app, base.Add(10*time.Minute))
	if err := writeFile(t, active, "live\n", base.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	linkCurrentLog(app, active)

	removed := pruneArchivedLogs(app, config.KeepLogCount())
	if removed != 4 {
		t.Fatalf("应清理 4 份旧归档，实际 %d", removed)
	}

	// 活日志必须还在且内容不变
	if data, err := os.ReadFile(active); err != nil || string(data) != "live\n" {
		t.Fatalf("正在写入的日志被破坏: %q %v", data, err)
	}
	if _, err := os.Lstat(active); err != nil {
		t.Fatalf("正在写入的日志被删除: %v", err)
	}

	sessions := ListLogSessions(app)
	if len(sessions) != 2 { // 1 归档 + 1 活
		t.Fatalf("会话数量错误: %d %v", len(sessions), sessionNames(sessions))
	}

	entries, _ := readDirNames(logsDir)
	t.Logf("目录内容: %v", entries)
}

// TestPruneDisabledWhenKeepCountNegative 保留份数为负数时不清理（不限制）
func TestPruneDisabledWhenKeepCountNegative(t *testing.T) {
	setupLogDir(t)
	app := "party-auth"

	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.Local)
	for i := 0; i < 4; i++ {
		start := base.Add(time.Duration(i) * time.Minute)
		path := NewSessionLogPath(app, start)
		if err := writeFile(t, path, "archived\n", start.Add(10*time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := archiveLogFile(path); err != nil {
			t.Fatal(err)
		}
	}

	for _, keep := range []int{-1, -100} {
		if removed := pruneArchivedLogs(app, keep); removed != 0 {
			t.Fatalf("keep=%d 时不应清理，实际清理 %d 份", keep, removed)
		}
	}
	if n := CountLogSessions(app); n != 4 {
		t.Fatalf("日志不应被清理: %d", n)
	}
}

// TestPruneZeroDeletesAllArchived keep=0 时归档全部清理，只留正在写入的日志
func TestPruneZeroDeletesAllArchived(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"

	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		start := base.Add(time.Duration(i) * time.Minute)
		path := NewSessionLogPath(app, start)
		if err := writeFile(t, path, "archived\n", start.Add(10*time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := archiveLogFile(path); err != nil {
			t.Fatal(err)
		}
	}

	active := NewSessionLogPath(app, base.Add(5*time.Minute))
	if err := writeFile(t, active, "live\n", base.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if removed := pruneArchivedLogs(app, 0); removed != 3 {
		t.Fatalf("keep=0 应清理 3 份归档，实际 %d", removed)
	}

	entries, err := readDirNames(logsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("只应保留正在写入的日志，实际: %v", entries)
	}
	if data, err := os.ReadFile(active); err != nil || string(data) != "live\n" {
		t.Fatalf("正在写入的日志被破坏: %q %v", data, err)
	}
}

// TestLogKeepCountDefaultsAndOverride 默认 7，.ini 可覆盖
func TestLogKeepCountDefaultsAndOverride(t *testing.T) {
	config.Init()
	if got := config.KeepLogCount(); got != 7 {
		t.Fatalf("默认保留份数应为 7，实际 %d", got)
	}

	// 模拟 .ini 中 application.log_keep_count = 15
	config.AppConfig.Application.LogKeepCount = 15
	if got := config.KeepLogCount(); got != 15 {
		t.Fatalf("配置覆盖失败: %d", got)
	}

	// 0 与负数必须原样返回（0 = 不留归档，负数 = 不限制）
	config.AppConfig.Application.LogKeepCount = 0
	if got := config.KeepLogCount(); got != 0 {
		t.Fatalf("keep=0 应原样返回，实际 %d", got)
	}
	config.AppConfig.Application.LogKeepCount = -1
	if got := config.KeepLogCount(); got != -1 {
		t.Fatalf("keep=-1 应原样返回，实际 %d", got)
	}

	config.AppConfig.Application.LogKeepCount = config.DefaultLogKeepCount
}

// TestArchiveThenPruneKeepsNewestFirst 归档后立即清理时，刚归档的那份必须保留
func TestArchiveThenPruneKeepsNewestFirst(t *testing.T) {
	setupLogDir(t)
	app := "party-auth"
	svc := NewApplicationService()
	config.AppConfig.Application.LogKeepCount = 2

	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.Local)

	var lastArchived string
	for i := 0; i < 5; i++ {
		start := base.Add(time.Duration(i) * time.Minute)
		session := NewSessionLogPath(app, start)
		if err := writeFile(t, session, fmt.Sprintf("run %d\n", i+1), start.Add(20*time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.archiveSessionLog(newApp(app, session)); err != nil {
			t.Fatal(err)
		}
		pruneArchivedLogs(app, config.KeepLogCount())
		lastArchived = filepath.Base(archivedSessionPath(session, start.Add(20*time.Second)))
	}

	// 最后归档的那份必须在
	if _, err := os.Lstat(filepath.Join(config.GetLogsDir(), lastArchived)); err != nil {
		t.Fatalf("最新归档被误删: %s %v", lastArchived, err)
	}
	if n := CountLogSessions(app); n != 2 {
		t.Fatalf("应保留 2 份，实际 %d", n)
	}
}
