package services

import (
	"app-manager-go-v1/internal/config"
	"app-manager-go-v1/internal/models"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLogrotateConfigMatchesSessionLogs 生成的 logrotate 配置必须能匹配新命名
func TestLogrotateConfigMatchesSessionLogs(t *testing.T) {
	config.Init()

	svc := NewLogService()
	content := svc.generateLogrotateConfig("/opt/party", "test")

	t.Logf("生成的配置:\n%s", content)

	wantRules := []string{
		"*-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]*.log",
		"*.log",
	}
	for _, want := range wantRules {
		if !contains(content, want) {
			t.Fatalf("logrotate 配置缺少规则: %s", want)
		}
	}

	// 新命名的会话日志与归档日志都应被第一条规则覆盖
	samples := []string{
		"party-auth-2026-09-16-161016启动.log",
		"party-auth-2026-09-16-161016启动-2026-09-16-161142停止.log",
		"party-auth-2026-09-16-161016.log",       // 旧命名兼容
		"party-auth-2026-09-16-161016-停止.log",   // 旧命名兼容
	}
	for _, sample := range samples {
		if !matchLogrotateGlob(wantRules[0], sample) {
			t.Fatalf("规则未匹配: %s", sample)
		}
	}

	// 旧版固定命名由第二条规则兜底
	if !matchLogrotateGlob(wantRules[1], "party-auth.log") {
		t.Fatal("旧版固定命名规则未匹配 party-auth.log")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// writeFile 写入日志文件并把 mtime 固定为给定时刻
func writeFile(t *testing.T, path, content string, mtime time.Time) error {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return os.Chtimes(path, mtime, mtime)
}

// setMtime 修改文件 mtime（模拟进程最后写入时刻）
func setMtime(t *testing.T, path string, mtime time.Time) error {
	t.Helper()
	return os.Chtimes(path, mtime, mtime)
}

// newApp 构造仅用于日志归档的 Application
func newApp(name, logFile string) *models.Application {
	return &models.Application{Name: name, LogFile: logFile}
}

// readDirNames 列出目录内所有条目名（含软链接）
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}

// matchLogrotateGlob 用 filepath.Match 模拟 logrotate 的通配（* 不跨 /）
func matchLogrotateGlob(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// TestRestartCycleNaming 模拟线上 restart 一轮：归档名同时体现启动与停止时刻
func TestRestartCycleNaming(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"

	svc := NewApplicationService()

	// 第 1 次启动 16:10:16
	start1 := time.Date(2026, 9, 16, 16, 10, 16, 0, time.Local)
	session1 := NewSessionLogPath(app, start1)
	if filepath.Base(session1) != "party-auth-2026-09-16-161016启动.log" {
		t.Fatalf("启动命名错误: %s", filepath.Base(session1))
	}
	if err := writeFile(t, session1, "run 1 output\n", start1); err != nil {
		t.Fatal(err)
	}
	linkCurrentLog(app, session1)

	// 16:11:42 停止
	stopTime := time.Date(2026, 9, 16, 16, 11, 42, 0, time.Local)
	if err := setMtime(t, session1, stopTime); err != nil {
		t.Fatal(err)
	}
	archived, err := svc.archiveSessionLog(newApp(app, session1))
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	if filepath.Base(archived) != "party-auth-2026-09-16-161016启动-2026-09-16-161142停止.log" {
		t.Fatalf("归档命名错误: %s", filepath.Base(archived))
	}

	// 同一次停止被重复调用：不得产生第二个归档文件
	again, err := svc.archiveSessionLog(newApp(app, session1))
	if err != nil {
		t.Fatalf("重复归档失败: %v", err)
	}
	if again != "" {
		t.Fatalf("重复停止不应再归档: %s", filepath.Base(again))
	}

	// 16:11:17 重新启动（restart 的新会话）
	start2 := time.Date(2026, 9, 16, 16, 11, 17, 0, time.Local)
	session2 := NewSessionLogPath(app, start2)
	if err := writeFile(t, session2, "run 2 output\n", start2); err != nil {
		t.Fatal(err)
	}
	linkCurrentLog(app, session2)

	// 目录内容检查
	entries, err := readDirNames(logsDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("目录内容: %v", entries)

	// <app>.log 软链接只在支持软链接的平台存在（Windows 无权限时自动跳过）
	symlinksSupported := false
	if info, err := os.Lstat(LegacyLogPath(app)); err == nil && info.Mode()&os.ModeSymlink != 0 {
		symlinksSupported = true
	}

	want := map[string]bool{
		"party-auth-2026-09-16-161016启动-2026-09-16-161142停止.log": true,
		"party-auth-2026-09-16-161117启动.log":                     true,
	}
	if symlinksSupported {
		want["party-auth.log"] = true
	}
	for _, name := range entries {
		if !want[name] {
			t.Fatalf("出现多余文件: %s（目录: %v）", name, entries)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("缺少文件: %v（目录: %v）", want, entries)
	}

	// 历史列表：归档的那次带启动+停止时间，当前这次只有启动时间
	sessions := ListLogSessions(app)
	if len(sessions) != 2 {
		t.Fatalf("会话数量错误: %d", len(sessions))
	}
	if !sessions[0].Stopped && sessions[0].StartTime.Equal(start2) {
		// sessions[0] 是较新的那次（16:11:17，未归档）
		if !sessions[0].StopTime.IsZero() {
			t.Fatalf("未归档会话不应有停止时间: %v", sessions[0].StopTime)
		}
	} else {
		t.Fatalf("最新会话应是 16:11:17 那次: %+v", sessions[0])
	}
	if !sessions[1].Stopped || !sessions[1].StopTime.Equal(stopTime) {
		t.Fatalf("归档会话停止时间错误: %+v", sessions[1])
	}
}
