package core

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Complete literal paths with SFTP; command text is never evaluated by a shell.
func (r *Remote) CompletePath(directory, word string, directoriesOnly bool) ([]string, error) {
	if len(word) > 4096 || strings.ContainsAny(word, "\x00\r\n") {
		return nil, fmt.Errorf("补全路径无效")
	}
	if strings.HasPrefix(word, "~") && word != "~" && !strings.HasPrefix(word, "~/") {
		return []string{}, nil
	}
	lookup := word
	if strings.HasPrefix(word, "~") {
		home, err := r.SFTP.Getwd()
		if err != nil {
			return nil, err
		}
		lookup = home + strings.TrimPrefix(word, "~")
		if word == "~" {
			return []string{"~/"}, nil
		}
	}
	if !path.IsAbs(lookup) {
		lookup = strings.TrimSuffix(directory, "/") + "/" + lookup
	}
	parent, prefix := path.Split(lookup)
	if strings.HasSuffix(word, "/") || word == "" {
		parent, prefix = lookup, ""
	}
	entries, err := r.SFTP.ReadDir(path.Clean(parent))
	if err != nil {
		return nil, err
	}
	before, _ := path.Split(word)
	out := []string{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) || (directoriesOnly && !entry.IsDir()) || (strings.HasPrefix(entry.Name(), ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		candidate := before + entry.Name()
		if entry.IsDir() {
			candidate += "/"
		}
		out = append(out, candidate)
	}
	sort.Strings(out)
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}
