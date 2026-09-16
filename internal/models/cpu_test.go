package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCPUPercentOfHost ps 的单核口径必须按核数归一化到整机口径
func TestCPUPercentOfHost(t *testing.T) {
	cores := CPUCount()
	if cores < 1 {
		t.Fatalf("CPUCount 异常: %d", cores)
	}
	t.Logf("当前机器 CPU 核数: %d", cores)

	cases := []struct {
		perCore float64
		want    float64
	}{
		{0, 0},
		{-5, 0},
		{100, 100.0 / float64(cores)},
		{float64(cores) * 100, 100}, // 吃满所有核 => 整机 100%
		{float64(cores) * 200, 100}, // 超过核数（采样抖动）也必须封顶 100%
	}

	for _, c := range cases {
		got := CPUPercentOfHost(c.perCore)
		if diff := got - c.want; diff > 0.0001 || diff < -0.0001 {
			t.Fatalf("CPUPercentOfHost(%.1f) = %.4f，期望 %.4f", c.perCore, got, c.want)
		}
	}

	// 关键回归: 单核口径超过 100% 时不再被截断成 100%，而是换算成合理的整机占比
	if cores > 1 {
		perCore := 150.0
		got := CPUPercentOfHost(perCore)
		if got >= 100 {
			t.Fatalf("%d 核机器上 150%%(单核) 不应换算成 %.1f%%", cores, got)
		}
		if got <= 0 {
			t.Fatalf("换算结果异常: %.2f", got)
		}
	}
}

// TestFormatCPU 展示格式: 单核口径 + 整机口径
func TestFormatCPU(t *testing.T) {
	cores := CPUCount()

	// 未超过 100%: 只显示单核口径
	if got := FormatCPUWithHost(80); got != "80.0%" {
		t.Fatalf("≤100%% 时格式错误: %s", got)
	}

	if got := FormatCPUPerCore(150); got != "150.0%" {
		t.Fatalf("单核口径格式错误: %s", got)
	}

	hostOnly := FormatCPUHost(150)
	if !strings.HasSuffix(hostOnly, "%") {
		t.Fatalf("整机口径格式错误: %s", hostOnly)
	}

	if cores > 1 {
		withHost := FormatCPUWithHost(150)
		if !strings.Contains(withHost, "150.0%") || !strings.Contains(withHost, "整机") {
			t.Fatalf(">100%% 时应同时给出整机口径: %s", withHost)
		}
		if !strings.Contains(withHost, "核") {
			t.Fatalf("说明中应包含核数: %s", withHost)
		}

		// 主展示（给 status 用的 CPUUsage）以整机口径开头
		main := FormatCPUHostWithPerCore(150)
		if !strings.HasPrefix(main, hostOnly) {
			t.Fatalf("主展示应以整机口径开头: %s", main)
		}
	}
}

// TestNormalizeCPUCount cgroup 限额收敛
func TestNormalizeCPUCount(t *testing.T) {
	cases := []struct {
		total int
		quota float64
		want  int
	}{
		{16, 0, 16},    // 无限额
		{16, 2, 2},     // 容器限 2 核
		{16, 2.5, 3},   // 2.5 核向上取整
		{16, 32, 16},   // 限额大于核数时取核数
		{0, 0, 1},      // 兜底
		{16, -1, 16},   // -1 表示不限制
	}

	for _, c := range cases {
		if got := normalizeCPUCount(c.total, c.quota); got != c.want {
			t.Fatalf("normalizeCPUCount(%d, %.1f) = %d，期望 %d", c.total, c.quota, got, c.want)
		}
	}
}

// TestCgroupCPUQuotaFrom 解析 cgroup v2 / v1 限额
func TestCgroupCPUQuotaFrom(t *testing.T) {
	// cgroup v2
	v2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(v2, "cpu.max"), []byte("200000 100000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := cgroupCPUQuotaFrom(v2); got != 2 {
		t.Fatalf("cgroup v2 限额解析错误: %v", got)
	}

	// cgroup v2 无限制
	v2max := t.TempDir()
	if err := os.WriteFile(filepath.Join(v2max, "cpu.max"), []byte("max 100000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := cgroupCPUQuotaFrom(v2max); got != 0 {
		t.Fatalf("cpu.max=max 应视为不限制，实际 %v", got)
	}

	// cgroup v1
	v1 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(v1, "cpu"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v1, "cpu", "cpu.cfs_quota_us"), []byte("150000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v1, "cpu", "cpu.cfs_period_us"), []byte("100000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := cgroupCPUQuotaFrom(v1); got != 1.5 {
		t.Fatalf("cgroup v1 限额解析错误: %v", got)
	}

	// 无 cgroup 文件 => 0（不限制）
	if got := cgroupCPUQuotaFrom(t.TempDir()); got != 0 {
		t.Fatalf("无 cgroup 文件应返回 0，实际 %v", got)
	}
}

// TestSystemCPUCountFromProc /proc/stat 核数统计（Linux 上用于自检）
func TestSystemCPUCountFromProc(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "stat")
	content := "cpu  1 2 3 4 5 6 7 8 9 10\n" +
		"cpu0 1 2 3 4 5 6 7 8 9 10\n" +
		"cpu1 1 2 3 4 5 6 7 8 9 10\n" +
		"cpu2 1 2 3 4 5 6 7 8 9 10\n" +
		"intr 0\n"
	if err := os.WriteFile(fake, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	original := cpuStatPath
	cpuStatPath = fake
	defer func() { cpuStatPath = original }()

	if got := SystemCPUCountFromProc(); got != 3 {
		t.Fatalf("/proc/stat 核数统计错误: %d", got)
	}
}
