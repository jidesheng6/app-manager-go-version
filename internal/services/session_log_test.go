package services

import (
	"app-manager-go-v1/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSessionLogNaming 验证会话日志命名/归档命名/解析逻辑
func TestSessionLogNaming(t *testing.T) {
	config.Init()

	start := time.Date(2025, 1, 1, 10, 15, 30, 0, time.Local)

	// 1. 启动时的会话日志命名: <app>-yyyy-MM-dd-HHmmss.log
	cur := NewSessionLogPath("party-auth", start)
	if filepath.Base(cur) != "party-auth-2025-01-01-101530.log" {
		t.Fatalf("会话命名错误: %s", filepath.Base(cur))
	}

	appName, parsedStart, stopped := parseSessionName(filepath.Base(cur))
	if appName != "party-auth" || stopped || !parsedStart.Equal(start) {
		t.Fatalf("会话解析错误: %q %v %v", appName, parsedStart, stopped)
	}

	// 2. 停止时归档: 保留启动时间戳，加 -停止 标记
	archived := archivedSessionPath(cur, time.Date(2025, 1, 1, 10, 15, 30, 0, time.Local))
	if filepath.Base(archived) != "party-auth-2025-01-01-101530-停止.log" {
		t.Fatalf("归档命名错误: %s", filepath.Base(archived))
	}

	// 3. 传入已归档文件时不再重复叠加 -停止
	again := archivedSessionPath(archived, time.Date(2025, 1, 1, 13, 0, 0, 0, time.Local))
	if filepath.Base(again) != "party-auth-2025-01-01-101530-停止.log" {
		t.Fatalf("二次归档命名错误: %s", filepath.Base(again))
	}

	// 4. 归档文件名解析
	name, s, st := parseSessionName("party-auth-2025-01-01-101530-停止.log")
	if name != "party-auth" || !st || !s.Equal(start) {
		t.Fatalf("归档解析错误: %q %v %v", name, s, st)
	}

	// 5. 应用名带版本号/多段连字符
	if got := sessionAppName("my-app-1.0.0-2025-01-01-101530.log"); got != "my-app-1.0.0" {
		t.Fatalf("带版本号应用名还原错误: %s", got)
	}
	if got := sessionAppName("my-app-1.0.0-2025-01-01-101530-停止.log"); got != "my-app-1.0.0" {
		t.Fatalf("带版本号归档应用名还原错误: %s", got)
	}
}

// TestArchiveLogFileFlow 验证归档、幂等与冲突处理
func TestArchiveLogFileFlow(t *testing.T) {
	dir := t.TempDir()

	start := "2025-01-01-101530"
	path := filepath.Join(dir, "party-auth-"+start+".log")
	if err := os.WriteFile(path, []byte("hello log\n"), 0644); err != nil {
		t.Fatal(err)
	}

	archived, err := archiveLogFile(path)
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}

	name, archivedStart, stopped := parseSessionName(filepath.Base(archived))
	if name != "party-auth" || !stopped || archivedStart.IsZero() {
		t.Fatalf("归档文件名应为 <app>-<停止时间>-停止.log，实际: %s", filepath.Base(archived))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("原文件应已被改名: %s", path)
	}
	if data, err := os.ReadFile(archived); err != nil || string(data) != "hello log\n" {
		t.Fatalf("归档内容丢失: %v", err)
	}

	// 二次归档 -> 原样返回（幂等）
	same, err := archiveLogFile(archived)
	if err != nil || same != archived {
		t.Fatalf("已归档文件二次归档异常: %s %v", same, err)
	}

	// 不存在文件 -> 返回空，不报错
	missing, err := archiveLogFile(filepath.Join(dir, "nope.log"))
	if err != nil || missing != "" {
		t.Fatalf("不存在文件处理异常: %q %v", missing, err)
	}
}

// TestNextArchivedPath 验证同名冲突时自动加序号
func TestNextArchivedPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a-2025-01-01-101530-停止.log")
	if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	got := nextArchivedPath(target)
	if filepath.Base(got) != "a-2025-01-01-101530-停止(2).log" {
		t.Fatalf("冲突处理错误: %s", filepath.Base(got))
	}
}
