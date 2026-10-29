//go:build !windows

package core

import "fmt"

func SetSecret(id, value string) error    { return fmt.Errorf("凭据保存仅支持 Windows") }
func GetSecret(id string) (string, error) { return "", fmt.Errorf("凭据读取仅支持 Windows") }

func DeleteSecret(id string) error { return nil }
