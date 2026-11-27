package models

import (
	"strings"
	"testing"
)

// TestParseCPUUnit 单位解析：默认 core，可配 m / percent，无法识别时回退 core
func TestParseCPUUnit(t *testing.T) {
	cases := map[string]CPUUnit{
		"":            CPUUnitCore,
		"core":        CPUUnitCore,
		"core ":       CPUUnitCore,
		"CORE":        CPUUnitCore,
		"核":           CPUUnitCore, // 无法识别 -> 回退默认
		"m":           CPUUnitMilli,
		"milli":       CPUUnitMilli,
		"millicore":   CPUUnitMilli,
		"millicores":  CPUUnitMilli,
		"M":           CPUUnitMilli,
		"percent":     CPUUnitPercent,
		"%":           CPUUnitPercent,
		"pct":         CPUUnitPercent,
		"ratio":       CPUUnitPercent,
		"percentage!": CPUUnitCore, // 未知值回退
	}

	for input, want := range cases {
		if got := ParseCPUUnit(input); got != want {
			t.Fatalf("ParseCPUUnit(%q) = %v，期望 %v", input, got, want)
		}
	}

	// 单位名称（用于表头）
	if CPUUnitCore.String() != "core" || CPUUnitMilli.String() != "m" || CPUUnitPercent.String() != "percent" {
		t.Fatalf("单位名称错误: %s/%s/%s", CPUUnitCore, CPUUnitMilli, CPUUnitPercent)
	}
}

// TestFormatCPUByUnit 三种单位下的展示
func TestFormatCPUByUnit(t *testing.T) {
	cores := CPUCount()

	// pcpu 125% => 1.25 核
	if got := FormatCPUByUnit(125, CPUUnitCore); !strings.HasPrefix(got, "1.25 core") {
		t.Fatalf("core 单位格式错误: %s", got)
	}
	if got := FormatCPUByUnit(125, CPUUnitMilli); got != "1250m" {
		t.Fatalf("m 单位格式错误: %s", got)
	}
	if got := FormatCPUByUnit(125, CPUUnitPercent); got != FormatCPUHost(125) {
		t.Fatalf("percent 单位格式错误: %s", got)
	}

	// 详情：三种单位都要能读出换算关系
	for _, unit := range []CPUUnit{CPUUnitCore, CPUUnitMilli, CPUUnitPercent} {
		detail := FormatCPUDetailByUnit(125, unit)
		if !strings.Contains(detail, "核") {
			t.Fatalf("%s 详情应包含核数: %s", unit, detail)
		}
		if unit != CPUUnitMilli && !strings.Contains(detail, "1250m") {
			t.Fatalf("%s 详情应包含 millicore: %s", unit, detail)
		}
	}

	// 合计展示
	if got := FormatCPUCoreTotal(2.5, CPUUnitCore); got != "2.50 core" {
		t.Fatalf("合计 core 格式错误: %s", got)
	}
	if got := FormatCPUCoreTotal(2.5, CPUUnitMilli); got != "2500m" {
		t.Fatalf("合计 m 格式错误: %s", got)
	}
	if got := FormatCPUCoreTotal(float64(cores), CPUUnitPercent); got != "100.0%" {
		t.Fatalf("吃满所有核的整机占比应为 100%%，实际 %s", got)
	}
}
