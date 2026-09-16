package models

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// CPU 使用量的口径说明
//
// ps 的 pcpu 是"占**单核**的百分比"：跑满 1 个核 = 100%，跑满 2 个核 = 200%。
// 多核服务器上该值会超过 100%（上限 = 核数 × 100%），早期版本直接截断到 100%，
// 既不准也丢信息。
//
// 现在对外统一用 **core（核）** 作为主单位，同时给出 millicore 精确值：
//
//	pcpu 100% => 1.00 core (1000m)
//	pcpu 250% => 2.50 core (2500m)
//
// core 是绝对量、与机器核数无关，多核高负载不会被截断，也能跨应用直接相加；
// 需要"占整机百分比"时再按核数换算：host% = core / 核数 × 100。

const (
	// MilliPerCore 一个完整核心对应的 millicore 数
	MilliPerCore = 1000
	// perCorePercentToCore ps 的 pcpu(占单核百分比) 换算成核数的系数
	perCorePercentToCore = 0.01
)

var (
	cpuCountOnce  sync.Once
	cpuCountValue int
)

// CPUCount 返回用于归一化的 CPU 核数
//
// 优先使用 cgroup 限额（容器里 NumCPU 会返回宿主机核数，直接用会让占用率被严重低估），
// 取不到时回退到 runtime.NumCPU()。
func CPUCount() int {
	cpuCountOnce.Do(func() {
		cpuCountValue = normalizeCPUCount(runtime.NumCPU(), cgroupCPUQuota())
	})

	return cpuCountValue
}

// cgroupCPUQuota 读取容器 CPU 限额（单位：核），读不到返回 0
func cgroupCPUQuota() float64 {
	return cgroupCPUQuotaFrom("/sys/fs/cgroup")
}

// cgroupCPUQuotaFrom 从指定 cgroup 根目录读取 CPU 限额（抽出来便于测试）
func cgroupCPUQuotaFrom(root string) float64 {
	// cgroup v2: cpu.max = "<quota> <period>" 或 "max <period>"
	if data, err := os.ReadFile(filepath.Join(root, "cpu.max")); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) == 2 && fields[0] != "max" {
			if quota, err := strconv.ParseFloat(fields[0], 64); err == nil {
				if period, err := strconv.ParseFloat(fields[1], 64); err == nil && period > 0 {
					return quota / period
				}
			}
		}
	}

	// cgroup v1: cpu.cfs_quota_us / cpu.cfs_period_us
	quotaData, err := os.ReadFile(filepath.Join(root, "cpu", "cpu.cfs_quota_us"))
	if err != nil {
		return 0
	}
	periodData, err := os.ReadFile(filepath.Join(root, "cpu", "cpu.cfs_period_us"))
	if err != nil {
		return 0
	}

	quota, err := strconv.ParseFloat(strings.TrimSpace(string(quotaData)), 64)
	if err != nil || quota <= 0 {
		return 0 // -1 表示不限制
	}
	period, err := strconv.ParseFloat(strings.TrimSpace(string(periodData)), 64)
	if err != nil || period <= 0 {
		return 0
	}

	return quota / period
}

// normalizeCPUCount 把原始核数按 cgroup 限额收敛（抽出来便于测试）
func normalizeCPUCount(total int, quota float64) int {
	if total < 1 {
		total = 1
	}
	if quota > 0 && quota < float64(total) {
		total = int(math.Ceil(quota))
	}
	return total
}

// CPUCore 把 ps 的单核口径百分比换算成核数
//
//	pcpu 100% => 1.0 核，pcpu 250% => 2.5 核
func CPUCore(perCorePercent float64) float64 {
	if perCorePercent <= 0 {
		return 0
	}
	return perCorePercent * perCorePercentToCore
}

// CPUUnit CPU 展示单位
type CPUUnit int

const (
	// CPUUnitCore 以核为单位（默认）: 1.25 core
	CPUUnitCore CPUUnit = iota
	// CPUUnitMilli 以 millicore 为单位: 1250m
	CPUUnitMilli
	// CPUUnitPercent 以整机占比为单位: 6.2%
	CPUUnitPercent
)

// ParseCPUUnit 解析配置中的单位名称，无法识别时回退到核数
func ParseCPUUnit(value string) CPUUnit {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "m", "milli", "millicore", "millicores":
		return CPUUnitMilli
	case "percent", "%", "pct", "ratio":
		return CPUUnitPercent
	default:
		return CPUUnitCore
	}
}

// String 单位名称
func (u CPUUnit) String() string {
	switch u {
	case CPUUnitMilli:
		return "m"
	case CPUUnitPercent:
		return "percent"
	default:
		return "core"
	}
}

// FormatCPUByUnit 按指定单位格式化 CPU 使用量
//   - core:    1.25 core（超过 1 核附精确值 2.50 core (2500m)）
//   - m:       1250m
//   - percent: 6.2%（占整机比例）
func FormatCPUByUnit(perCorePercent float64, unit CPUUnit) string {
	switch unit {
	case CPUUnitMilli:
		return FormatCPUMilli(perCorePercent)
	case CPUUnitPercent:
		return FormatCPUHost(perCorePercent)
	default:
		return FormatCPUWithHost(perCorePercent)
	}
}

// FormatCPUDetailByUnit 详情展示（含换算过程），按指定单位
func FormatCPUDetailByUnit(perCorePercent float64, unit CPUUnit) string {
	core := CPUCore(perCorePercent)

	switch unit {
	case CPUUnitMilli:
		return fmt.Sprintf("%s (%s, 整机 %s / %d核)",
			FormatCPUMilliFromCore(core), FormatCPUCoreFromCore(core),
			FormatCPUHostFromCore(core), CPUCount())
	case CPUUnitPercent:
		return fmt.Sprintf("%s (%.2f core / %s, %d核)",
			FormatCPUHostFromCore(core), core, FormatCPUMilliFromCore(core), CPUCount())
	default:
		return FormatCPUDetail(perCorePercent)
	}
}

// FormatCPUCoreTotal 合计/均值展示，按指定单位（传入的已是核数）
func FormatCPUCoreTotal(core float64, unit CPUUnit) string {
	switch unit {
	case CPUUnitMilli:
		return FormatCPUMilliFromCore(core)
	case CPUUnitPercent:
		return FormatCPUHostFromCore(core)
	default:
		return FormatCPUCoreFromCore(core)
	}
}

// CPUUnitHint 单位说明文本（给 status 系统信息行用）
func CPUUnitHint() string {
	return "CPU使用量单位 core: 1.00 core = 1 个核, 可在 .ini 中配置 application.cpu_unit=core|m|percent"
}

// CPUMilli 把 ps 的单核口径百分比换算成 millicore（1000m = 1 核）
//
//	pcpu 100% => 1000m，pcpu 250% => 2500m
func CPUMilli(perCorePercent float64) float64 {
	return CPUCore(perCorePercent) * MilliPerCore
}

// FormatCPU 主展示单位: 核数，如 "2.50"、"0.08"
func FormatCPU(perCorePercent float64) string {
	return fmt.Sprintf("%.2f", CPUCore(perCorePercent))
}

// FormatCPUCore 带单位: 如 "2.50 core"
func FormatCPUCore(perCorePercent float64) string {
	return fmt.Sprintf("%.2f core", CPUCore(perCorePercent))
}

// FormatCPUMilli millicore 精确值，如 "2500m"、"80m"
func FormatCPUMilli(perCorePercent float64) string {
	return fmt.Sprintf("%.0fm", CPUMilli(perCorePercent))
}

// FormatCPUCoreFromCore 已是核数时的展示
func FormatCPUCoreFromCore(core float64) string {
	return fmt.Sprintf("%.2f core", core)
}

// FormatCPUMilliFromCore 已是核数时的 millicore 展示
func FormatCPUMilliFromCore(core float64) string {
	return fmt.Sprintf("%.0fm", core*MilliPerCore)
}

// FormatCPUHostFromCore 核数换算整机占比，如 "15.6%"
func FormatCPUHostFromCore(core float64) string {
	cores := CPUCount()
	if cores < 1 {
		cores = 1
	}

	host := core / float64(cores) * 100
	if host > 100 {
		host = 100 // 极端情况（ps 采样抖动）兜底
	}
	return fmt.Sprintf("%.1f%%", host)
}

// FormatCPUDetail 详情展示: 核数 + millicore + 整机占比
//
//	"2.50 core (2500m, 整机 15.6% / 16核)"
func FormatCPUDetail(perCorePercent float64) string {
	core := CPUCore(perCorePercent)
	return fmt.Sprintf("%s (%s, 整机 %s / %d核)",
		FormatCPUCoreFromCore(core),
		FormatCPUMilliFromCore(core),
		FormatCPUHostFromCore(core),
		CPUCount())
}

// FormatCPUWithHost 列表展示: 核数为主；超过 1 个核时附 millicore
//
//	"0.80 core" / "2.50 core (2500m)"
func FormatCPUWithHost(perCorePercent float64) string {
	core := CPUCore(perCorePercent)
	if core <= 1 {
		return FormatCPUCoreFromCore(core)
	}
	return fmt.Sprintf("%s (%s)", FormatCPUCoreFromCore(core), FormatCPUMilliFromCore(core))
}

// FormatCPUCoreWithHost 列表展示（核数 + 整机占比），用于需要百分比的场景
func FormatCPUCoreWithHost(perCorePercent float64) string {
	core := CPUCore(perCorePercent)
	return fmt.Sprintf("%s (整机 %s)", FormatCPUCoreFromCore(core), FormatCPUHostFromCore(core))
}

// CPUPercentOfHost 把 ps 的单核口径百分比换算成整机口径（0~100，保底不超过 100）
func CPUPercentOfHost(perCorePercent float64) float64 {
	if perCorePercent <= 0 {
		return 0
	}

	cores := CPUCount()
	if cores < 1 {
		cores = 1
	}

	host := perCorePercent / float64(cores)
	if host > 100 {
		host = 100 // 极端情况（ps 采样抖动）兜底
	}
	return host
}

// FormatCPUPerCore 单核口径百分比展示，如 "150.0%"（保留给需要百分比的场景）
func FormatCPUPerCore(perCorePercent float64) string {
	if perCorePercent <= 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", perCorePercent)
}

// FormatCPUHost 整机占比展示，如 "9.4%"
func FormatCPUHost(perCorePercent float64) string {
	return fmt.Sprintf("%.1f%%", CPUPercentOfHost(perCorePercent))
}

// cpuStatPath 便于测试替换 /proc 路径
var cpuStatPath = "/proc/stat"

// SystemCPUCountFromProc 从 /proc/stat 统计逻辑核数（仅用于自检对比 CPUCount）
func SystemCPUCountFromProc() int {
	data, err := os.ReadFile(cpuStatPath)
	if err != nil {
		return 0
	}

	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// 只统计 cpuN 行，跳过汇总行 cpu
		if !strings.HasPrefix(fields[0], "cpu") || fields[0] == "cpu" {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err == nil {
			count++
		}
	}

	return count
}
