//go:build !windows

package main

func (a *App) GetClipboardFiles() ([]string, error) { return []string{}, nil }
