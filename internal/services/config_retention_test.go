package services

import (
	"app-manager-go-v1/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLogKeepCountFromIni 验证 .ini 中的 application.log_keep_count 能被正确读取
func TestLogKeepCountFromIni(t *testing.T) {
	projectRoot := t.TempDir()
	backendDir := filepath.Join(projectRoot, "project_backend")
	if err := os.MkdirAll(filepath.Join(backendDir, "logs"), 0755); err != nil {
		t.Fatal(err)
	}

	iniContent := `[global]
project_root="` + projectRoot + `"
java_path="java"

[application]
app_type="jar"
log_keep_count=15
`
	iniPath := filepath.Join(backendDir, ".am.ini")
	if err := os.WriteFile(iniPath, []byte(iniContent), 0644); err != nil {
		t.Fatal(err)
	}

	config.Init()
	if err := config.LoadConfig(iniPath); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if got := config.KeepLogCount(); got != 15 {
		t.Fatalf("log_keep_count=15 应生效，实际 %d", got)
	}

	// 清理: 恢复默认，避免影响其他用例
	config.AppConfig.Application.LogKeepCount = config.DefaultLogKeepCount
}

// TestInitTemplateContainsLogKeepCount init 生成的配置模板必须带默认值 7
func TestInitTemplateContainsLogKeepCount(t *testing.T) {
	config.Init()

	svc := NewProjectService()
	template := svc.getApplicationConfigTemplate()

	if !strings.Contains(template, "log_keep_count={{LOG_KEEP_COUNT}}") {
		t.Fatal("配置模板缺少 log_keep_count 占位符")
	}

	rendered := strings.ReplaceAll(template, "{{LOG_KEEP_COUNT}}", itoa(config.KeepLogCount()))
	if !strings.Contains(rendered, "log_keep_count=7") {
		t.Fatalf("默认值应为 7，实际渲染: %s", rendered)
	}

	// log_keep_count 的占位符必须被替换掉（其他占位符由 init 流程统一替换）
	if strings.Contains(rendered, "{{LOG_KEEP_COUNT}}") {
		t.Fatal("log_keep_count 占位符未替换")
	}
}

// TestPruneKeepsConfiguredNumberOfSessions 10 份归档 + 保留 7 => 只留最近 7 份
func TestPruneKeepsConfiguredNumberOfSessions(t *testing.T) {
	logsDir := setupLogDir(t)
	app := "party-auth"
	config.AppConfig.Application.LogKeepCount = config.DefaultLogKeepCount // 7

	// 与线上一致的归档命名
	files := []string{
		"party-auth-2026-09-16-010000启动-2026-09-16-013000停止.log",
		"party-auth-2026-09-16-020000启动-2026-09-16-023000停止.log",
		"party-auth-2026-09-16-030000启动-2026-09-16-033000停止.log",
		"party-auth-2026-09-16-040000启动-2026-09-16-043000停止.log",
		"party-auth-2026-09-16-050000启动-2026-09-16-053000停止.log",
		"party-auth-2026-09-16-060000启动-2026-09-16-063000停止.log",
		"party-auth-2026-09-16-070000启动-2026-09-16-073000停止.log",
		"party-auth-2026-09-16-080000启动-2026-09-16-083000停止.log",
		"party-auth-2026-09-16-090000启动-2026-09-16-093000停止.log",
		"party-auth-2026-09-16-100000启动-2026-09-16-103000停止.log",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(logsDir, name), []byte("log\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if n := CountLogSessions(app); n != 10 {
		t.Fatalf("前置条件失败: %d", n)
	}

	removed := pruneArchivedLogs(app, config.KeepLogCount())
	if removed != 3 {
		t.Fatalf("应清理 3 份，实际 %d", removed)
	}

	remaining, err := readDirNames(logsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 7 {
		t.Fatalf("应保留 7 份，实际 %d: %v", len(remaining), remaining)
	}

	// 保留的必须是最新的 7 份（04:00 起）
	for _, name := range remaining {
		if strings.Contains(name, "-0100") || strings.Contains(name, "-0200") || strings.Contains(name, "-0300") {
			t.Fatalf("最旧的日志应被清理: %s", name)
		}
	}
	for _, want := range files[3:] {
		if _, err := os.Lstat(filepath.Join(logsDir, want)); err != nil {
			t.Fatalf("应保留的日志被删除: %s", want)
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
