package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var Version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("mehshell", Version)
			return
		case "init":
			if len(os.Args) > 2 && os.Args[2] == "zsh" {
				cfg := loadConfig()
				fmt.Println(zshInitScript(cfg))
			} else {
				fmt.Fprintln(os.Stderr, "usage: mehshell init zsh")
				os.Exit(1)
			}
			return
		case "config":
			if len(os.Args) > 2 && os.Args[2] == "init" {
				p := configPath()
				if _, err := os.Stat(p); err == nil {
					fmt.Fprintln(os.Stderr, "config already exists:", p)
					os.Exit(1)
				}
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					fmt.Fprintln(os.Stderr, "error:", err)
					os.Exit(1)
				}
				if err := os.WriteFile(p, []byte(defaultConfigFile), 0644); err != nil {
					fmt.Fprintln(os.Stderr, "error:", err)
					os.Exit(1)
				}
				fmt.Println("created", p)
			} else if len(os.Args) > 2 && os.Args[2] == "path" {
				fmt.Println(configPath())
			} else {
				fmt.Fprintln(os.Stderr, "usage: mehshell config [init|path]")
				os.Exit(1)
			}
			return
		}
	}

	cfg := loadConfig()
	on := cfg.Segments

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

	add := func(dst *[]seg, order int, bg int, fn func() string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if t := fn(); t != "" {
				mu.Lock()
				*dst = append(*dst, seg{t, order, bg})
				mu.Unlock()
			}
		}()
	}

	icons := cfg.Icons
	if on["os"] {
		add(&left, 0, 24, func() string { return segOS(icons) })
	}
	if on["dir"] {
		add(&left, 1, 31, func() string { return segDir(cwd, home) })
	}
	if on["git"] {
		add(&left, 2, 97, func() string { return segGit(cwd, icons) })
	}
	if on["node"] {
		add(&left, 3, 34, func() string { return segNode(cwd, icons) })
	}
	if on["python"] {
		add(&left, 4, 136, func() string { return segPython(cwd, icons) })
	}
	if on["go"] {
		add(&left, 5, 37, func() string { return segGo(cwd, icons) })
	}
	if on["rust"] {
		add(&left, 6, 130, func() string { return segRust(cwd, icons) })
	}
	if on["ruby"] {
		add(&left, 7, 124, func() string { return segRuby(cwd, icons) })
	}
	if on["java"] {
		add(&left, 8, 94, func() string { return segJava(cwd, icons) })
	}
	if on["conda"] {
		add(&right, 0, 34, func() string { return segConda(icons) })
	}
	if on["venv"] {
		add(&right, 1, 34, func() string { return segVenv(icons) })
	}
	if on["k8s"] {
		add(&right, 2, 25, func() string { return segK8s(cwd, home, icons) })
	}
	if on["terraform"] {
		add(&right, 3, 57, func() string { return segTerraform(cwd, icons) })
	}
	if on["docker"] {
		add(&right, 4, 25, func() string { return segDocker(cwd, icons) })
	}
	if on["aws"] {
		add(&right, 5, 166, func() string { return segAWS(icons) })
	}
	if on["azure"] {
		add(&right, 6, 25, func() string { return segAzure(icons) })
	}
	if on["gcloud"] {
		add(&right, 7, 34, func() string { return segGCloud(icons) })
	}
	if on["battery"] {
		add(&right, 8, 22, func() string { return segBattery(icons) })
	}
	if on["duration"] {
		add(&right, 9, 136, func() string { return segDuration(duration) })
	}
	if on["time"] {
		add(&right, 10, 238, func() string { return segTime() })
	}

	wg.Wait()

	sort.Slice(left, func(i, j int) bool { return left[i].order < left[j].order })
	sort.Slice(right, func(i, j int) bool { return right[i].order < right[j].order })

	var line1, line2 string

	charColor := cGreen
	if exitCode != 0 {
		charColor = cRed
	}

	switch cfg.Style {
	case "classic", "rainbow":
		rainbow := cfg.Style == "rainbow"
		lStr := renderPowerlineLeft(left, rainbow)
		rStr := renderPowerlineRight(right, rainbow)
		pad := columns - visibleWidth(lStr) - visibleWidth(rStr)
		if pad < 1 {
			pad = 1
		}
		line1 = lStr + strings.Repeat(" ", pad) + rStr
		line2 = fc(charColor, "❯")
	default:
		leftStr := joinSegs(left, " ")
		rightStr := joinSegs(right, "  ")
		prefix := fc(cCyan, "╭─") + " "
		prefixVis := 3
		pad := columns - prefixVis - visibleWidth(leftStr) - visibleWidth(rightStr)
		if pad < 1 {
			pad = 1
		}
		line1 = prefix + leftStr + strings.Repeat(" ", pad) + rightStr
		line2 = fc(cCyan, "╰─") + fc(charColor, "❯")
	}

	l1 := escShell(line1)
	l2 := escShell(line2)
	fmt.Printf("PROMPT=$'\\n%s\\n%s '\n", l1, l2)
}
