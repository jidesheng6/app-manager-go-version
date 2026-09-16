package services

import (
	"app-manager-go-v1/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSessionLogNaming 验证会话/归档命名与解析
// 命名规则: <app>-<启动时间>启动.log / <app>-<启动时间>启动-<停止时间>停止.log
func TestSessionLogNaming(t *testing.T) {
	config.Init()

	start := time.Date(2025, 1, 1, 10, 15, 30, 0, time.Local)
	stop := time.Date(2025, 1, 1, 18, 30, 0, 0, time.Local)

	// 1. 启动时的会话日志命名
	cur := NewSessionLogPath("party-auth", start)
	if filepath.Base(cur) != "party-auth-2025-01-01-101530启动.log" {
		t.Fatalf("会话命名错误: %s", filepath.Base(cur))
	}

	session, zero := parseSessionName(filepath.Base(cur))
	if zero || session.AppName != "party-auth" || session.Stopped ||
		!session.StartTime.Equal(start) || !session.StopTime.IsZero() {
		t.Fatalf("会话解析错误: %+v zero=%v", session, zero)
	}

	// 2. 停止归档命名: 启动时间 + 停止时间
	archived := archivedSessionPath(cur, stop)
	if filepath.Base(archived) != "party-auth-2025-01-01-101530启动-2025-01-01-183000停止.log" {
		t.Fatalf("归档命名错误: %s", filepath.Base(archived))
	}

	session, zero = parseSessionName(filepath.Base(archived))
	if zero || session.AppName != "party-auth" || !session.Stopped ||
		!session.StartTime.Equal(start) || !session.StopTime.Equal(stop) {
		t.Fatalf("归档解析错误: %+v zero=%v", session, zero)
	}

	// 3. 同秒停止时停止时间顺延 1 秒，避免两个时间戳完全相同
	sameSecond := archivedSessionPath(cur, start)
	wantName := "party-auth-2025-01-01-101530启动-2025-01-01-101531停止.log"
	if filepath.Base(sameSecond) != wantName {
		t.Fatalf("同秒归档命名错误: %s", filepath.Base(sameSecond))
	}

	// 4. 再次归档已归档文件: 仍生成合法归档名（不叠加 -停止 标记、不产生停止(2) 副本）
	relocated := archivedSessionPath(archived, time.Date(2025, 1, 2, 9, 0, 0, 0, time.Local))
	relocatedSession, relocatedZero := parseSessionName(filepath.Base(relocated))
	if relocatedZero || !relocatedSession.Stopped {
		t.Fatalf("再次归档应生成合法归档名: %s", filepath.Base(relocated))
	}
	if relocatedSession.AppName != "party-auth" {
		t.Fatalf("再次归档应用名错误: %s", relocatedSession.AppName)
	}
	if strings.Contains(filepath.Base(relocated), "停止(2)") {
		t.Fatalf("不应产生停止(2) 冲突副本: %s", filepath.Base(relocated))
	}

	// 5. 归档标记识别
	if !hasStoppedMark(filepath.Base(archived)) {
		t.Fatal("归档文件应被识别为已停止")
	}
	if hasStoppedMark(filepath.Base(cur)) {
		t.Fatal("会话文件不应被识别为已停止")
	}

	// 6. 应用名还原（含版本号、多段连字符）
	if got := sessionAppName("my-app-1.0.0-2025-01-01-101530启动.log"); got != "my-app-1.0.0" {
		t.Fatalf("带版本号应用名还原错误: %s", got)
	}
	if got := sessionAppName("my-app-1.0.0-2025-01-01-101530启动-2025-01-01-183000停止.log"); got != "my-app-1.0.0" {
		t.Fatalf("带版本号归档应用名还原错误: %s", got)
	}
	if got := sessionAppName("party-auth.log"); got != "party-auth" {
		t.Fatalf("旧版固定命名还原错误: %s", got)
	}
}

// TestLegacySessionNamingSupported 上一版命名仍然可识别、可归档
func TestLegacySessionNamingSupported(t *testing.T) {
	config.Init()

	cases := []struct {
		fileName string
		appName  string
		start    time.Time
		stopped  bool
		wantZero bool
	}{
		{"party-auth-2026-09-16-161016.log", "party-auth", time.Date(2026, 9, 16, 16, 10, 16, 0, time.Local), false, false},
		{"party-auth-2026-09-16-161016-停止.log", "party-auth", time.Date(2026, 9, 16, 16, 10, 16, 0, time.Local), true, false},
		{"party-auth-2026-09-16-154954-停止(2).log", "party-auth", time.Date(2026, 9, 16, 15, 49, 54, 0, time.Local), true, false},
		{"party-auth.log", "", time.Time{}, false, true},
	}

	for _, c := range cases {
		session, zero := parseSessionName(c.fileName)
		if zero != c.wantZero {
			t.Fatalf("%s: zero=%v 期望 %v", c.fileName, zero, c.wantZero)
		}
		if c.wantZero {
			continue
		}
		if !sessionIsForApp(session, c.appName) {
			t.Fatalf("%s: 应归属应用 %s，实际 %s", c.fileName, c.appName, session.AppName)
		}
		if session.Stopped != c.stopped {
			t.Fatalf("%s: stopped=%v 期望 %v", c.fileName, session.Stopped, c.stopped)
		}
		if !session.StartTime.Equal(c.start) {
			t.Fatalf("%s: 启动时间 %v 期望 %v", c.fileName, session.StartTime, c.start)
		}
	}
}

// TestArchiveLogFileFlow 归档真实文件、幂等、冲突处理
func TestArchiveLogFileFlow(t *testing.T) {
	dir := t.TempDir()

	start := "2025-01-01-101530"
	path := filepath.Join(dir, "party-auth-"+start+"启动.log")
	if err := os.WriteFile(path, []byte("hello log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, writeTime, writeTime); err != nil {
		t.Fatal(err)
	}

	archived, err := archiveLogFile(path)
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}

	session, zero := parseSessionName(filepath.Base(archived))
	if zero || session.AppName != "party-auth" || !session.Stopped {
		t.Fatalf("归档文件名不符合 <app>-<启动>启动-<停止>停止.log: %s", filepath.Base(archived))
	}
	if !session.StartTime.Equal(time.Date(2025, 1, 1, 10, 15, 30, 0, time.Local)) {
		t.Fatalf("归档应保留原启动时间，实际 %v", session.StartTime)
	}
	if !session.StopTime.Equal(writeTime) {
		t.Fatalf("归档停止时间应取内容最后写入时间 %v，实际 %v", writeTime, session.StopTime)
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

// TestNextArchivedPath 同名冲突时自动加序号
func TestNextArchivedPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a-2025-01-01-101530启动-2025-01-01-120000停止.log")
	if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	got := nextArchivedPath(target)
	if filepath.Base(got) != "a-2025-01-01-101530启动-2025-01-01-120000停止(2).log" {
		t.Fatalf("冲突处理错误: %s", filepath.Base(got))
	}
}
