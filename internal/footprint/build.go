package footprint

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"claude-memory-sync/internal/domain"
)

const memoryFile = "MEMORY.md"

func Build(workDir string, now time.Time) (Report, error) {
	info, err := os.Stat(workDir)
	if errors.Is(err, fs.ErrNotExist) {
		return Report{}, domain.Error(domain.ErrNotFound, "work dir not found", slog.String("work_dir", workDir))
	}
	if err != nil {
		return Report{}, domain.Error(err, "stat work dir", slog.String("work_dir", workDir))
	}
	if !info.IsDir() {
		return Report{}, domain.Error(domain.ErrNotFound, "work dir is not a directory", slog.String("work_dir", workDir))
	}

	projects, err := buildProjects(workDir)
	if err != nil {
		return Report{}, domain.WrapError(err, slog.String("work_dir", workDir))
	}
	global, err := buildGlobal(workDir)
	if err != nil {
		return Report{}, domain.WrapError(err, slog.String("work_dir", workDir))
	}

	return Report{
		GeneratedAt: now,
		Limits:      Limits{MemoryLines: MemoryLinesLimit, MemoryBytes: MemoryBytesLimit, WarnPct: WarnPct},
		Projects:    projects,
		Global:      global,
	}, nil
}

func readDir(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.Error(err, "read dir", slog.String("dir", dir))
	}
	return entries, nil
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.Error(err, "lstat file", slog.String("path", path))
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, domain.Error(err, "read file", slog.String("path", path))
	}
	return data, nil
}

func buildProjects(workDir string) ([]Project, error) {
	root := filepath.Join(workDir, "projects")
	dirs, err := readDir(root)
	if err != nil {
		return nil, err
	}

	projects := []Project{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		memDir := filepath.Join(root, d.Name(), "memory")
		memInfo, err := os.Lstat(memDir)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !memInfo.IsDir()) {
			continue
		}
		if err != nil {
			return nil, domain.Error(err, "lstat memory dir", slog.String("path", memDir))
		}
		project, err := buildProject(d.Name(), memDir)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].PctOfLimit != projects[j].PctOfLimit {
			return projects[i].PctOfLimit > projects[j].PctOfLimit
		}
		return projects[i].Key < projects[j].Key
	})
	return projects, nil
}

func buildProject(key, memDir string) (Project, error) {
	index, err := readRegular(filepath.Join(memDir, memoryFile))
	if err != nil {
		return Project{}, err
	}
	files, err := readDir(memDir)
	if err != nil {
		return Project{}, err
	}

	entries := 0
	for _, f := range files {
		if f.Type().IsRegular() && f.Name() != memoryFile && strings.HasSuffix(f.Name(), ".md") {
			entries++
		}
	}

	lines := bytes.Count(index, []byte("\n"))
	if len(index) > 0 && index[len(index)-1] != '\n' {
		lines++
	}
	return Project{
		Key:         key,
		MemoryLines: lines,
		MemoryBytes: len(index),
		Entries:     entries,
		PctOfLimit:  max(lines*100/MemoryLinesLimit, len(index)*100/MemoryBytesLimit),
	}, nil
}

func buildGlobal(workDir string) (Global, error) {
	claudeMD, err := readRegular(filepath.Join(workDir, "global", "CLAUDE.md"))
	if err != nil {
		return Global{}, err
	}
	global, err := buildRules(filepath.Join(workDir, "global", "rules"))
	if err != nil {
		return Global{}, err
	}
	global.ClaudeMDBytes = len(claudeMD)
	global.Skills, err = countSkills(filepath.Join(workDir, "global", "skills"))
	if err != nil {
		return Global{}, err
	}
	return global, nil
}

func buildRules(rulesDir string) (Global, error) {
	global := Global{RulesUnconditional: []string{}, RulesScoped: []string{}}
	rules, err := readDir(rulesDir)
	if err != nil {
		return Global{}, err
	}
	for _, r := range rules {
		if !r.Type().IsRegular() || !strings.HasSuffix(r.Name(), ".md") {
			continue
		}
		data, err := readRegular(filepath.Join(rulesDir, r.Name()))
		if err != nil {
			return Global{}, err
		}
		if hasPathsFrontmatter(data) {
			global.RulesScoped = append(global.RulesScoped, r.Name())
			global.RulesScopedBytes += len(data)
			continue
		}
		global.RulesUnconditional = append(global.RulesUnconditional, r.Name())
		global.RulesUnconditionalBytes += len(data)
	}
	sort.Strings(global.RulesScoped)
	sort.Strings(global.RulesUnconditional)
	return global, nil
}

func countSkills(skillsDir string) (int, error) {
	skills, err := readDir(skillsDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, s := range skills {
		if !s.IsDir() {
			continue
		}
		skillMD, err := readRegular(filepath.Join(skillsDir, s.Name(), "SKILL.md"))
		if err != nil {
			return 0, err
		}
		if skillMD != nil {
			count++
		}
	}
	return count, nil
}

func hasPathsFrontmatter(data []byte) bool {
	lines := strings.Split(string(data), "\n")
	if strings.TrimRight(lines[0], "\r") != "---" {
		return false
	}
	hasPaths := false
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "---" {
			return hasPaths
		}
		if strings.HasPrefix(line, "paths:") {
			hasPaths = true
		}
	}
	return false
}
