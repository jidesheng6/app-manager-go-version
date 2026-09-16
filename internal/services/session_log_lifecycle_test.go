package services

import (
	"app-manager-go-v1/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLogLifecycleE2E 端到端验证日志留痕流程：
// 启动 -> 运行(写入) -> 异常退出补归档 -> 再次启动 -> 停止归档 -> 历史会话列举
func TestLogLifecycleE2E(t *testing.T) {
	projectRoot := t.TempDir()
	backendDir := filepath.Join(projectRoot, "project_backend")
	logsDir := filepath.Join(backendDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 用测试项目根目录重新初始化配置
	config.Init()
	config.AppConfig.Global.ProjectRoot = projectRoot
	config.ConfigFile = filepath.Join(projectRoot, ".app-manager.ini")
	if err := os.WriteFile(config.ConfigFile, []byte("[global]\nproject_root = "+projectRoot+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := config.GetLogsDir(); got != logsDir {
		t.Fatalf("日志目录推导错误: %s", got)
	}

	app := "party-auth"

	// === 第一次启动：新建会话日志 ===
	first := NewSessionLogPath(app, time.Date(2025, 3, 1, 9, 0, 0, 0, time.Local))
	if err := os.WriteFile(first, []byte("boot 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	linkCurrentLog(app, first)

	if cur := ResolveCurrentLog(app); filepath.Base(cur) != filepath.Base(first) {
		t.Fatalf("解析当前日志错误: %s", cur)
	}
	if n := CountLogSessions(app); n != 1 {
		t.Fatalf("会话数错误: %d", n)
	}

	// === 模拟被 kill -9：文件未归档，下次启动前应补归档 ===
	if stopped, _ := os.Stat(first); stopped == nil {
		// 修改内容时间，确保归档时间戳稳定
		_ = os.Chtimes(first, time.Now(), time.Date(2025, 3, 1, 10, 30, 0, 0, time.Local))
	}
	archivedIllegal, err := archiveLogFile(first)
	if err != nil {
		t.Fatalf("补归档失败: %v", err)
	}
	if !hasStoppedMark(filepath.Base(archivedIllegal)) {
		t.Fatalf("补归档命名错误: %s", filepath.Base(archivedIllegal))
	}

	// === 第二次启动：再加一个会话 ===
	second := NewSessionLogPath(app, time.Date(2025, 3, 2, 9, 0, 0, 0, time.Local))
	if err := os.WriteFile(second, []byte("boot 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	linkCurrentLog(app, second)

	if cur := ResolveCurrentLog(app); filepath.Base(cur) != filepath.Base(second) {
		t.Fatalf("当前日志应指向最新会话: %s", cur)
	}

	// === 正常停止：归档并重建软链接 ===
	stoppedPath, err := archiveLogFile(second)
	if err != nil {
		t.Fatalf("停止归档失败: %v", err)
	}
	linkCurrentLog(app, stoppedPath)
	if !hasStoppedMark(filepath.Base(stoppedPath)) {
		t.Fatalf("归档命名缺少 -停止: %s", filepath.Base(stoppedPath))
	}

	// === 历史会话列举：两次运行都在，且新的在前 ===
	sessions := ListLogSessions(app)
	if len(sessions) != 2 {
		t.Fatalf("会话数量错误: %d (%v)", len(sessions), sessions)
	}
	if !sessions[0].StartTime.After(sessions[1].StartTime) {
		t.Fatalf("会话排序错误: %v > %v", sessions[0].StartTime, sessions[1].StartTime)
	}
	if !sessions[0].Stopped || !sessions[1].Stopped {
		t.Fatal("两个会话都应处于已归档状态")
	}

	// 原始日志内容未被覆盖（留痕的核心目标）
	data, err := os.ReadFile(sessions[0].Path)
	if err != nil || string(data) != "boot 2\n" {
		t.Fatalf("最新会话内容异常: %q %v", data, err)
	}
	data, err = os.ReadFile(sessions[1].Path)
	if err != nil || string(data) != "boot 1\n" {
		t.Fatalf("最早会话内容被覆盖: %q %v", data, err)
	}

	// 停止后 ResolveCurrentLog 仍应可用（指向最近归档文件）
	if cur := ResolveCurrentLog(app); cur == "" {
		t.Fatal("停止后应仍能解析到最近日志")
	}
}

// TestLegacyLogMigration 旧命名 <app>.log 的迁移归档
func TestLegacyLogMigration(t *testing.T) {
	projectRoot := t.TempDir()
	logsDir := filepath.Join(projectRoot, "project_backend", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}
	config.Init()
	config.AppConfig.Global.ProjectRoot = projectRoot

	app := "legacy-app"
	legacy := LegacyLogPath(app)
	if err := os.WriteFile(legacy, []byte("old session\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 旧文件存在时 ResolveCurrentLog 优先返回它
	if cur := ResolveCurrentLog(app); filepath.Base(cur) != app+".log" {
		t.Fatalf("旧日志应被优先解析: %s", cur)
	}

	archived, err := ArchiveLegacyLog(app)
	if err != nil {
		t.Fatalf("旧日志归档失败: %v", err)
	}
	if archived == "" || !hasStoppedMark(filepath.Base(archived)) {
		t.Fatalf("旧日志归档命名错误: %s", archived)
	}
	if data, _ := os.ReadFile(archived); string(data) != "old session\n" {
		t.Fatalf("旧日志内容异常: %q", data)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("旧文件应已被改名: %s", legacy)
	}

	// 空文件不产生归档垃圾
	emptyApp := "empty-app"
	if err := os.WriteFile(LegacyLogPath(emptyApp), nil, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ArchiveLegacyLog(emptyApp)
	if err != nil || got != "" {
		t.Fatalf("空日志不应归档: %q %v", got, err)
	}
}
