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

// CPU 使用率的口径说明
//
// ps 的 pcpu 是"占**单核**的百分比"：一个进程跑满 1 个核 = 100%，
// 跑满 2 个核 = 200%。所以多核服务器上这个值会超过 100%（上限 = 核数 × 100%）。
// 之前把它直接截断到 100%，多核高负载时看起来永远只有 100%，既不准确也丢信息。
//
// 现在统一用两个口径：
//   - 单核口径(per-core)：ps 原值，0 ~ 核数×100%，能看出吃满了几个核
//   - 整机口径(host)    ：per-core / 核数，0 ~ 100%，和 top 的整机占用一致，可跨应用相加

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

// CPUPercentOfHost 把 ps 的单核口径百分比换算成整机口径（0~100）
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

// FormatCPUPerCore 单核口径展示，如 "150.0%"
func FormatCPUPerCore(perCorePercent float64) string {
	if perCorePercent <= 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", perCorePercent)
}

// FormatCPUHost 整机口径展示，如 "9.4%"
func FormatCPUHost(perCorePercent float64) string {
	return fmt.Sprintf("%.1f%%", CPUPercentOfHost(perCorePercent))
}

// FormatCPUWithHost 同时给出两个口径，如 "150.0% (16核 → 整机 9.4%)"
// 单核占比未超过 100% 时只返回单核口径，避免信息冗余
func FormatCPUWithHost(perCorePercent float64) string {
	perCore := FormatCPUPerCore(perCorePercent)
	if perCorePercent <= 100 {
		return perCore
	}
	return fmt.Sprintf("%s (%d核 → 整机 %s)", perCore, CPUCount(), FormatCPUHost(perCorePercent))
}

// FormatCPUHostWithPerCore 整机口径在前，用于主展示（保证不超过 100%）
func FormatCPUHostWithPerCore(perCorePercent float64) string {
	host := FormatCPUHost(perCorePercent)
	if perCorePercent <= 100 {
		return host
	}
	return fmt.Sprintf("%s (单核 %s)", host, FormatCPUPerCore(perCorePercent))
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
