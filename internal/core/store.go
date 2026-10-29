package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func WriteJSON(filename string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return WriteAtomic(filename, func(f *os.File) error { _, e := f.Write(b); return e })
}

func WriteAtomic(filename string, write func(*os.File) error) error {
	filename, e := filepath.Abs(filename)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(filename), 0700); e != nil {
		return e
	}
	filename, e = atomicPath(filename)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(filename), ".config-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = write(f); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = replaceFile(tmp, filename); e != nil {
		return fmt.Errorf("保存配置 %s: %w", filename, e)
	}
	return nil
}
func LoadConfig(dir string) (Config, error) {
	c := EmptyConfig()
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(e, os.ErrNotExist) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	if e == nil {
		for i, w := range c.Workflows {
			c.Workflows[i] = NormalizeWorkflow(w)
		}
	}
	return c, e
}
