package services

import (
	"app-manager-go-v1/internal/config"
	"os"
	"path/filepath"
	"testing"
)

// TestCPUUnitDefaultsToCore 默认单位必须是 core：
// 没配置文件、配置里没有该键、值为空、值无法识别，四种情况都要回落到 core。
func TestCPUUnitDefaultsToCore(t *testing.T) {
	if config.DefaultCPUUnit != "core" {
		t.Fatalf("默认单位应为 core，实际 %q", config.DefaultCPUUnit)
	}

	// 1. 内置默认配置
	config.Init()
	if got := config.GetCPUUnit(); got != "core" {
		t.Fatalf("内置默认应为 core，实际 %q", got)
	}

	// 2. 配置里显式留空
	config.AppConfig.Application.CPUUnit = ""
	if got := config.GetCPUUnit(); got != "core" {
		t.Fatalf("空值应回落到 core，实际 %q", got)
	}

	// 3. .ini 里没有 cpu_unit 这一项
	projectRoot := t.TempDir()
	backendDir := filepath.Join(projectRoot, "project_backend")
	if err := os.MkdirAll(filepath.Join(backendDir, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	iniPath := filepath.Join(backendDir, ".am.ini")
	iniContent := "[global]\nproject_root=\"" + projectRoot + "\"\njava_path=\"java\"\n\n[application]\napp_type=\"jar\"\n"
	if err := os.WriteFile(iniPath, []byte(iniContent), 0644); err != nil {
		t.Fatal(err)
	}

	config.Init()
	if err := config.LoadConfig(iniPath); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got := config.GetCPUUnit(); got != "core" {
		t.Fatalf("ini 未配置时应为 core，实际 %q", got)
	}

	// 4. .ini 里确实配置了 m
	iniContent += "cpu_unit=\"m\"\n"
	if err := os.WriteFile(iniPath, []byte(iniContent), 0644); err != nil {
		t.Fatal(err)
	}
	config.Init()
	if err := config.LoadConfig(iniPath); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got := config.GetCPUUnit(); got != "m" {
		t.Fatalf("ini 配置 m 应生效，实际 %q", got)
	}

	// 收尾：恢复默认
	config.Init()
}
