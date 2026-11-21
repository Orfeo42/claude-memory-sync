package prompts

import (
	"embed"
	"log/slog"
	"path"
	"sort"
	"strings"

	"claude-memory-sync/internal/domain"
)

//go:embed *.md
var files embed.FS

func Names() []string {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, strings.TrimSuffix(entry.Name(), path.Ext(entry.Name())))
	}
	sort.Strings(names)
	return names
}

func Default(name string) (string, error) {
	data, err := files.ReadFile(name + ".md")
	if err != nil {
		return "", domain.Error(domain.ErrNotFound, "unknown prompt", slog.String("name", name))
	}
	return string(data), nil
}
