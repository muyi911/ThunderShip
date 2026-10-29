package core

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// Query only IDs present in the directory; getent also resolves LDAP/NSS accounts.
func (r *Remote) OwnerNames(ctx context.Context, entries []os.FileInfo) map[string]string {
	users, groups := map[uint32]bool{}, map[uint32]bool{}
	for _, entry := range entries {
		if stat, ok := entry.Sys().(*sftp.FileStat); ok {
			users[stat.UID], groups[stat.GID] = true, true
		}
	}
	if len(users) == 0 {
		return nil
	}
	ids := func(values map[uint32]bool) string {
		list := make([]string, 0, len(values))
		for id := range values {
			list = append(list, strconv.FormatUint(uint64(id), 10))
		}
		sort.Strings(list)
		return strings.Join(list, " ")
	}
	command := "if command -v getent >/dev/null 2>&1; then getent passwd " + ids(users) +
		"; else cat /etc/passwd; fi 2>/dev/null | awk -F: '{print $1 \":\" $3}'\nprintf '__THUNDERSHIP_GROUPS__\\n'\n" +
		"if command -v getent >/dev/null 2>&1; then getent group " + ids(groups) +
		"; else cat /etc/group; fi 2>/dev/null | awk -F: '{print $1 \":\" $3}'"
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var output strings.Builder
	err := r.Run(ctx, Step{Command: command}, func(text string) {
		if remaining := 1024*1024 - output.Len(); remaining > 0 {
			output.WriteString(text[:min(len(text), remaining)])
		}
	})
	if err != nil {
		return nil
	}
	return parseOwnerNames(output.String(), entries)
}

func parseOwnerNames(text string, entries []os.FileInfo) map[string]string {
	users, groups := map[uint32]string{}, map[uint32]string{}
	current := users
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "__THUNDERSHIP_GROUPS__" {
			current = groups
			continue
		}
		name, id, ok := strings.Cut(strings.TrimSpace(line), ":")
		value, err := strconv.ParseUint(id, 10, 32)
		if ok && err == nil && name != "" {
			current[uint32(value)] = name
		}
	}
	names := map[string]string{}
	for _, entry := range entries {
		if stat, ok := entry.Sys().(*sftp.FileStat); ok {
			user, group := users[stat.UID], groups[stat.GID]
			if user == "" {
				user = fmt.Sprint(stat.UID)
			}
			if group == "" {
				group = fmt.Sprint(stat.GID)
			}
			names[fmt.Sprintf("%d/%d", stat.UID, stat.GID)] = user + "/" + group
		}
	}
	return names
}
