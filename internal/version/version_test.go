package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo 建一个临时 git 仓库并提交一个文件；git 不可用时跳过
func initRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("环境没有 git: %v", err)
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v 失败: %v %s", args, err, out)
		}
	}

	run("init")
	run("checkout", "-b", "feat/version-test")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "test commit")

	return dir
}

// TestLiveAtReadsRepo 运行期读取当前目录 git 信息
func TestLiveAtReadsRepo(t *testing.T) {
	dir := initRepo(t)

	info := LiveAt(dir)
	if info == nil {
		t.Fatal("LiveAt 应返回仓库信息")
	}

	if info.Commit == "" || len(info.Commit) < 4 {
		t.Fatalf("提交号异常: %q", info.Commit)
	}
	if info.Branch != "feat/version-test" {
		t.Fatalf("分支读取错误: %q", info.Branch)
	}
	if !info.DirtyKnown {
		t.Fatal("应能判断工作区是否干净")
	}
	if info.Dirty {
		t.Fatalf("刚提交完应是干净的: %+v", info)
	}
	if info.CommitTime == "" {
		t.Fatal("应读取提交时间")
	}

	// 制造未提交改动
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dirty := LiveAt(dir)
	if dirty == nil || !dirty.Dirty {
		t.Fatalf("有未提交改动时应标记 dirty: %+v", dirty)
	}
}

// TestLiveAtNonRepo 未注入且不在 git 仓库内时不能崩，返回 nil。
// 注意: git 会在父目录里找 .git，所以"非仓库"必须找一个没有仓库祖先的目录；
// 找不到这样的目录时只断言不 panic。
func TestLiveAtNonRepo(t *testing.T) {
	if info := LiveAt(""); info != nil {
		t.Fatalf("空目录应返回 nil，实际: %+v", info)
	}
	if info := LiveAt(filepath.Join(t.TempDir(), "not-exist-dir")); info != nil {
		t.Fatalf("不存在的目录应返回 nil，实际: %+v", info)
	}

	// 找到一个祖先里没有 .git 的目录
	dir := t.TempDir()
	for probe := dir; ; {
		if _, err := os.Stat(filepath.Join(probe, ".git")); err == nil {
			t.Skipf("临时目录 %s 位于 git 仓库内，git 会向上查找，跳过该断言", dir)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}

	if info := LiveAt(dir); info != nil {
		t.Fatalf("非 git 目录应返回 nil，实际: %+v", info)
	}
}

// TestLiveAtInsideRepoSubdir git 向上查找父目录仓库属于预期行为
func TestLiveAtInsideRepoSubdir(t *testing.T) {
	dir := initRepo(t)

	sub := filepath.Join(dir, "sub", "deeper")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	info := LiveAt(sub)
	if info == nil {
		t.Fatal("子目录应能解析到父目录的 git 仓库")
	}
	if info.Commit == "" {
		t.Fatalf("应返回提交号: %+v", info)
	}
}

// TestRepoLineFormat 展示格式
func TestRepoLineFormat(t *testing.T) {
	if got := RepoLinePlain(nil); !strings.Contains(got, "未找到 git 仓库") {
		t.Fatalf("nil 情况提示错误: %s", got)
	}

	clean := &Info{Commit: "abc1234", Branch: "main", Describe: "v1.0.1-3-gabc1234", CommitTime: "2026-09-16 16:00:00"}
	got := RepoLinePlain(clean)
	for _, want := range []string{"main@abc1234", "v1.0.1-3-gabc1234", "干净", "提交于"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RepoLinePlain 缺少 %q: %s", want, got)
		}
	}
	if strings.Contains(got, "\033[") {
		t.Fatalf("Plain 版本不应带颜色码: %s", got)
	}

	dirty := &Info{Commit: "abc1234", Branch: "main", Dirty: true, DirtyKnown: true}
	if got := RepoLinePlain(dirty); !strings.Contains(got, "有未提交改动") {
		t.Fatalf("dirty 标记缺失: %s", got)
	}
}

// TestBuildLine BuildLine 展示构建期注入信息
func TestBuildLine(t *testing.T) {
	originalCommit, originalBranch, originalDirty, originalBuild := GitCommit, GitBranch, GitDirty, BuildTime
	defer func() {
		GitCommit, GitBranch, GitDirty, BuildTime = originalCommit, originalBranch, originalDirty, originalBuild
	}()

	GitCommit = "9b1c2d3"
	GitBranch = "feat/log-retention"
	GitDirty = "true"
	BuildTime = "2026-09-16T16:36:40"

	got := BuildLine("v1.0.2")
	for _, want := range []string{"v1.0.2", "feat/log-retention@9b1c2d3", "工作区有未提交改动", "2026-09-16T16:36:40"} {
		if !strings.Contains(got, want) {
			t.Fatalf("BuildLine 缺少 %q: %s", want, got)
		}
	}

	// 没注入 git 信息时给出可操作提示
	GitCommit, GitBranch, GitDirty, BuildTime = "", "", "", ""
	got = BuildLine("v1.0.2")
	if !strings.Contains(got, "未注入 git 信息") {
		t.Fatalf("缺少未注入提示: %s", got)
	}
}

// TestDriftHint 二进制与当前目录提交不一致时给出提示
func TestDriftHint(t *testing.T) {
	embedded := Info{Commit: "aaaaaaa"}
	same := &Info{Commit: "aaaaaaa"}
	if hint := DriftHint(embedded, same); hint != "" {
		t.Fatalf("提交一致时不应提示: %s", hint)
	}

	diff := &Info{Commit: "bbbbbbb"}
	hint := DriftHint(embedded, diff)
	if !strings.Contains(hint, "aaaaaaa") || !strings.Contains(hint, "bbbbbbb") {
		t.Fatalf("漂移提示应包含两个提交号: %s", hint)
	}

	if hint := DriftHint(Info{}, diff); hint != "" {
		t.Fatalf("未注入时不应提示: %s", hint)
	}
	if hint := DriftHint(embedded, nil); hint != "" {
		t.Fatalf("无仓库信息时不应提示: %s", hint)
	}
}

// TestDescribeToCommit 从 git describe 提取提交号
func TestDescribeToCommit(t *testing.T) {
	cases := map[string]string{
		"v1.0.1-3-g10980d9": "10980d9",
		"10980d9":           "",
		"v1.0.0":            "",
	}
	for input, want := range cases {
		if got := DescribeToCommit(input); got != want {
			t.Fatalf("DescribeToCommit(%q) = %q，期望 %q", input, got, want)
		}
	}
}
