package version

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// 构建期信息（由 -ldflags -X 注入，见 build-linux.ps1 / build-windows.ps1）
//
//	-X app-manager-go-v1/internal/version.GitCommit=...
//	-X app-manager-go-v1/internal/version.GitBranch=...
//	-X app-manager-go-v1/internal/version.GitDescribe=...
//	-X app-manager-go-v1/internal/version.GitDirty=...
//	-X app-manager-go-v1/internal/version.BuildTime=...
var (
	GitCommit   = ""
	GitBranch   = ""
	GitDescribe = ""
	GitDirty    = ""
	BuildTime   = ""
)

// Info 一份完整的版本/来源信息
type Info struct {
	Version     string // 版本号（main.Version）
	Commit      string // 提交号（短）
	Branch      string // 分支
	Describe    string // git describe（tag-N-gsha 或短 sha）
	Dirty       bool   // 构建时工作区是否有未提交改动
	DirtyKnown  bool   // 是否拿到了 dirty 信息
	Source      string // embedded=来自构建期注入，live=来自当前目录 git 仓库
	BuildTime   string // 构建时间
	CommitTime  string // 提交时间（live 可用时）
	LiveCommit  string // 当前 git 仓库的提交号（与 Commit 不一致说明有版本漂移）
}

var (
	liveOnce sync.Once
	liveInfo *Info
)

// Embedded 返回构建期注入的信息（无 git 信息时 Commit 为空）
func Embedded() Info {
	dirty := GitDirty == "true" || GitDirty == "1"
	return Info{
		Commit:     GitCommit,
		Branch:     GitBranch,
		Describe:   GitDescribe,
		Dirty:      dirty,
		DirtyKnown: GitDirty != "",
		Source:     "embedded",
		BuildTime:  BuildTime,
	}
}

// Available 构建期是否注入了 git 信息
func (i Info) Available() bool {
	return i.Commit != "" || i.Describe != ""
}

// Short 返回最短可读标识：优先 tag 描述，其次提交号
func (i Info) Short() string {
	if i.Describe != "" {
		return i.Describe
	}
	if i.Commit != "" {
		return i.Commit
	}
	return "unknown"
}

// DirtyMark 未提交改动的标记
func (i Info) DirtyMark() string {
	if i.Dirty {
		return "-dirty"
	}
	return ""
}

// LiveAt 读取指定目录的 git 仓库信息（只读操作，失败返回 nil）
//
//	git -C <dir> rev-parse --short HEAD
//	git -C <dir> rev-parse --abbrev-ref HEAD
//	git -C <dir> describe --tags --always
//	git -C <dir> status --porcelain
//	git -C <dir> log -1 --format=%cd
func LiveAt(dir string) *Info {
	if dir == "" {
		return nil
	}

	commit := gitOutput(dir, "rev-parse", "--short", "HEAD")
	if commit == "" {
		return nil // 不是 git 仓库或 git 不可用
	}

	info := &Info{
		Commit:    commit,
		LiveCommit: commit,
		Branch:    gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"),
		Describe:  gitOutput(dir, "describe", "--tags", "--always"),
		CommitTime: gitOutput(dir, "log", "-1", "--format=%cd", "--date=format:%Y-%m-%d %H:%M:%S"),
		Source:    "live",
	}

	// 有未提交改动（含未跟踪文件之外的修改）
	if status := gitOutput(dir, "status", "--porcelain"); status != "" {
		info.Dirty = true
	}
	info.DirtyKnown = true

	return info
}

// Live 返回当前进程可访问目录的 git 信息（进程只读一次并缓存）
func Live(dir string) *Info {
	liveOnce.Do(func() {
		liveInfo = LiveAt(dir)
	})
	return liveInfo
}

// gitOutput 执行只读 git 命令并返回单行输出（失败返回空字符串）
func gitOutput(dir string, args ...string) string {
	fullArgs := append([]string{"-C", dir}, args...)

	cmd := exec.Command("git", fullArgs...)
	cmd.Env = append(cmd.Environ(),
		"GIT_OPTIONAL_LOCKS=0", // 只读，避免写 index.lock
		"GIT_TERMINAL_PROMPT=0",
	)

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// DescribeToCommit 从 git describe 结果里提取提交号（形如 v1.0.1-3-g10980d9）
var describeCommitRegex = regexp.MustCompile(`-g([0-9a-f]{4,40})$`)

func DescribeToCommit(describe string) string {
	if m := describeCommitRegex.FindStringSubmatch(describe); m != nil {
		return m[1]
	}
	return ""
}

// BuildTimeOrNow 构建时间，缺失时用当前时间兜底（仅用于展示占位）
func BuildTimeOrNow() string {
	if BuildTime != "" {
		return BuildTime
	}
	return time.Now().Format("2006-01-02 15:04:05")
}

// BuildLine 一行描述二进制自身的来源：v1.0.1 (feat/log-retention@10980d9, 构建于 ...)
//
// 只使用**构建期注入**的信息，不做运行期 git 查询，
// 因此部署到没有 .git 的服务器上也能正常显示提交号。
//
// versionText 传入 main.Version（避免本包依赖 cmd 包）
func BuildLine(versionText string) string {
	embedded := Embedded()

	parts := []string{}
	if embedded.Branch != "" {
		parts = append(parts, embedded.Branch+"@"+embedded.Commit)
	} else if embedded.Commit != "" {
		parts = append(parts, embedded.Commit)
	}
	if embedded.Dirty {
		parts = append(parts, "工作区有未提交改动")
	}
	if embedded.BuildTime != "" {
		parts = append(parts, "构建于 "+embedded.BuildTime)
	}

	if len(parts) == 0 {
		return versionText + " (未注入 git 信息，建议用 build-linux.ps1 或带 ldflags 构建)"
	}

	return fmt.Sprintf("%s (%s)", versionText, strings.Join(parts, ", "))
}

// RepoLine 一行描述当前目录 git 仓库状态：分支@提交 (tag) 干净/有未提交改动
// useColor=false 时输出纯文本（便于脚本解析/重定向）
func RepoLine(info *Info) string {
	return repoLine(info, true)
}

// RepoLinePlain 无颜色版本
func RepoLinePlain(info *Info) string {
	return repoLine(info, false)
}

func repoLine(info *Info, useColor bool) string {
	if info == nil {
		return "未找到 git 仓库（该目录不是 git 工作区，或未安装 git）"
	}

	text := ""
	if info.Branch != "" {
		text = info.Branch + "@" + info.Commit
	} else {
		text = info.Commit
	}

	if info.Describe != "" && info.Describe != info.Commit {
		text += " (" + info.Describe + ")"
	}

	if info.Dirty {
		if useColor {
			text += " \033[33m有未提交改动\033[0m"
		} else {
			text += " [有未提交改动]"
		}
	} else {
		if useColor {
			text += " \033[32m干净\033[0m"
		} else {
			text += " [干净]"
		}
	}

	if info.CommitTime != "" {
		text += " 提交于 " + info.CommitTime
	}

	return text
}

// DriftHint 二进制与当前目录是否来自不同提交（版本漂移提示）
func DriftHint(embedded Info, live *Info) string {
	if live == nil || embedded.Commit == "" || live.Commit == "" {
		return ""
	}
	if embedded.Commit == live.Commit {
		return ""
	}
	return fmt.Sprintf("二进制由 %s 构建，当前目录是 %s —— 建议重新编译后再操作",
		embedded.Commit, live.Commit)
}
