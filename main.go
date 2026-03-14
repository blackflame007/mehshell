package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var Version = "dev"

// ── Zsh prompt colors ────────────────────────────────────────────

const (
	cCyan    = 75
	cBlue    = 39
	cMagenta = 170
	cGreen   = 76
	cRed     = 196
	cYellow  = 220
	cOrange  = 208
	cGray    = 242
)

func fc(color int, s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("%%F{%d}%s%%f", color, s)
}

// ── Segment with sort order ──────────────────────────────────────

type seg struct {
	text  string
	order int
}

// ── Main ─────────────────────────────────────────────────────────

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("mehshell", Version)
		return
	}

	exitCode := 0
	duration := 0
	columns := 80

	if len(os.Args) > 1 {
		exitCode, _ = strconv.Atoi(os.Args[1])
	}
	if len(os.Args) > 2 {
		duration, _ = strconv.Atoi(os.Args[2])
	}
	if len(os.Args) > 3 {
		columns, _ = strconv.Atoi(os.Args[3])
	}
	if columns <= 0 {
		columns = 80
	}

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var left, right []seg

	add := func(dst *[]seg, order int, fn func() string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if t := fn(); t != "" {
				mu.Lock()
				*dst = append(*dst, seg{t, order})
				mu.Unlock()
			}
		}()
	}

	// Left segments
	add(&left, 0, func() string { return segOS() })
	add(&left, 1, func() string { return segDir(cwd, home) })
	add(&left, 2, func() string { return segGit(cwd) })
	add(&left, 3, func() string { return segNode(cwd) })
	add(&left, 4, func() string { return segPython(cwd) })
	add(&left, 5, func() string { return segGo(cwd) })

	// Right segments
	add(&right, 0, func() string { return segConda() })
	add(&right, 1, func() string { return segVenv() })
	add(&right, 2, func() string { return segK8s(cwd, home) })
	add(&right, 3, func() string { return segAWS() })
	add(&right, 4, func() string { return segDuration(duration) })
	add(&right, 5, func() string { return segTime() })

	wg.Wait()

	sort.Slice(left, func(i, j int) bool { return left[i].order < left[j].order })
	sort.Slice(right, func(i, j int) bool { return right[i].order < right[j].order })

	leftStr := joinSegs(left, " ")
	rightStr := joinSegs(right, "  ")

	// Right-align line 1: pad between left and right
	prefix := fc(cCyan, "╭─") + " "
	prefixVis := 3 // "╭─ "
	leftVis := visibleWidth(leftStr)
	rightVis := visibleWidth(rightStr)
	pad := columns - prefixVis - leftVis - rightVis
	if pad < 1 {
		pad = 1
	}

	line1 := prefix + leftStr + strings.Repeat(" ", pad) + rightStr

	// Prompt char
	char := fc(cGreen, "❯")
	if exitCode != 0 {
		char = fc(cRed, "❯")
	}
	line2 := fc(cCyan, "╰─") + char

	// Escape for $'...' quoting
	l1 := escShell(line1)
	l2 := escShell(line2)

	fmt.Printf("PROMPT=$'\\n%s\\n%s '\n", l1, l2)
}

// ── Segments ─────────────────────────────────────────────────────

func segOS() string {
	icon := "\uf17c" // linux tux
	switch runtime.GOOS {
	case "linux":
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			lower := strings.ToLower(string(data))
			switch {
			case strings.Contains(lower, "arch"):
				icon = "\uf303"
			case strings.Contains(lower, "ubuntu"):
				icon = "\uf31b"
			case strings.Contains(lower, "fedora"):
				icon = "\uf30a"
			case strings.Contains(lower, "debian"):
				icon = "\uf306"
			case strings.Contains(lower, "nixos"):
				icon = "\uf313"
			}
		}
	case "darwin":
		icon = "\uf179"
	}
	return fc(cBlue, icon)
}

func segDir(cwd, home string) string {
	dir := cwd
	if strings.HasPrefix(dir, home) {
		dir = "~" + dir[len(home):]
	}
	if dir == "~/" {
		dir = "~"
	}
	return fc(cBlue, dir)
}

func segGit(cwd string) string {
	branch, repoDir := gitBranch(cwd)
	if branch == "" {
		return ""
	}

	dirty := gitDirty(repoDir)
	result := fc(cBlue, "\uf126 ") + fc(cMagenta, branch)
	if dirty != "" {
		result += " " + dirty
	}
	return result
}

func segNode(cwd string) string {
	if _, err := os.Stat(filepath.Join(cwd, "package.json")); err != nil {
		return ""
	}
	// Try reading version files first (no fork)
	for _, f := range []string{".node-version", ".nvmrc"} {
		if data, err := os.ReadFile(filepath.Join(cwd, f)); err == nil {
			ver := strings.TrimSpace(string(data))
			if ver != "" {
				return fc(cGreen, "\ue718 "+ver)
			}
		}
	}
	// Fallback: run node --version
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "node", "--version").Output(); err == nil {
		return fc(cGreen, "\ue718 "+strings.TrimSpace(string(out)))
	}
	return ""
}

func segPython(cwd string) string {
	if !hasMarkerUp(cwd, []string{".python-version", "pyproject.toml", "setup.py", "Pipfile", "requirements.txt"}) {
		return ""
	}
	if data, err := os.ReadFile(filepath.Join(cwd, ".python-version")); err == nil {
		ver := strings.TrimSpace(string(data))
		if ver != "" {
			return fc(cYellow, "\ue73c "+ver)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "python", "--version").Output(); err == nil {
		ver := strings.TrimPrefix(strings.TrimSpace(string(out)), "Python ")
		return fc(cYellow, "\ue73c "+ver)
	}
	return ""
}

func segGo(cwd string) string {
	gomod := filepath.Join(cwd, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "go ") {
			ver := strings.Fields(line)[1]
			return fc(cCyan, "\ue724 "+ver)
		}
	}
	return ""
}

func segConda() string {
	env := os.Getenv("CONDA_DEFAULT_ENV")
	if env == "" || env == "base" {
		return ""
	}
	return fc(cGreen, "\ue73c "+env)
}

func segVenv() string {
	venv := os.Getenv("VIRTUAL_ENV")
	if venv == "" {
		return ""
	}
	return fc(cGreen, "\ue73c "+filepath.Base(venv))
}

func segK8s(cwd, home string) string {
	markers := []string{"skaffold.yaml", "helmfile.yaml", "Chart.yaml", "kustomization.yaml"}
	hasMarker := false
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(cwd, m)); err == nil {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return ""
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	data, err := os.ReadFile(kubeconfig)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "current-context:") {
			ctx := strings.TrimSpace(strings.TrimPrefix(line, "current-context:"))
			if ctx != "" && ctx != "\"\"" {
				return fc(cBlue, "\u2388 "+ctx)
			}
		}
	}
	return ""
}

func segAWS() string {
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		return ""
	}
	return fc(cOrange, "\uf52c "+profile)
}

func segDuration(secs int) string {
	if secs < 3 {
		return ""
	}
	var s string
	switch {
	case secs >= 3600:
		s = fmt.Sprintf("%dh%dm", secs/3600, (secs%3600)/60)
	case secs >= 60:
		s = fmt.Sprintf("%dm%ds", secs/60, secs%60)
	default:
		s = fmt.Sprintf("%ds", secs)
	}
	return fc(cYellow, s)
}

func segTime() string {
	return fc(cGray, time.Now().Format("03:04:05 PM"))
}

// ── Git helpers ──────────────────────────────────────────────────

func gitBranch(cwd string) (branch string, repoDir string) {
	dir := cwd
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			var headPath string
			if info.IsDir() {
				headPath = filepath.Join(gitPath, "HEAD")
			} else {
				// Worktree/submodule: .git is a file
				data, err := os.ReadFile(gitPath)
				if err != nil {
					return "", ""
				}
				gitDir := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir: "))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(dir, gitDir)
				}
				headPath = filepath.Join(gitDir, "HEAD")
			}
			data, err := os.ReadFile(headPath)
			if err != nil {
				return "", ""
			}
			head := strings.TrimSpace(string(data))
			if strings.HasPrefix(head, "ref: refs/heads/") {
				return head[16:], dir
			}
			if len(head) >= 8 {
				return head[:8], dir
			}
			return "", ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

func gitDirty(repoDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		// Timeout or error — show branch only, no lag
		return ""
	}

	staged, modified, untracked := false, false, false
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		switch {
		case x == '?':
			untracked = true
		default:
			if x != ' ' {
				staged = true
			}
			if y != ' ' {
				modified = true
			}
		}
	}

	if !staged && !modified && !untracked {
		return fc(cGreen, "✓")
	}

	var parts []string
	if staged {
		parts = append(parts, fc(cGreen, "+"))
	}
	if modified {
		parts = append(parts, fc(cRed, "!"))
	}
	if untracked {
		parts = append(parts, fc(cYellow, "?"))
	}
	return strings.Join(parts, "")
}

// ── Helpers ──────────────────────────────────────────────────────

func hasMarkerUp(cwd string, markers []string) bool {
	dir := cwd
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

func visibleWidth(s string) int {
	// Strip zsh prompt escapes: %F{N}...%f, %B, %b, etc.
	stripped := s
	for {
		idx := strings.Index(stripped, "%F{")
		if idx == -1 {
			break
		}
		end := strings.Index(stripped[idx:], "}")
		if end == -1 {
			break
		}
		stripped = stripped[:idx] + stripped[idx+end+1:]
	}
	stripped = strings.ReplaceAll(stripped, "%f", "")
	stripped = strings.ReplaceAll(stripped, "%%", "%")
	return utf8.RuneCountInString(stripped)
}

func escShell(s string) string {
	// Escape for zsh $'...' quoting
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func joinSegs(segs []seg, sep string) string {
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = s.text
	}
	return strings.Join(parts, sep)
}
