package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shell_workflow/internal/core"
)

func TestTerminalAIUsesConfiguredModelAndOnlyReturnsSuggestion(t *testing.T) {
	var received string
	var receivedRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Error(r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["model"] != "test-model" {
			t.Error(request["model"])
		}
		data, _ := json.Marshal(request)
		received = string(data)
		receivedRequest = request
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"command":"ls -la","explanation":"列出目录"}`}}}})
	}))
	defer server.Close()
	app := NewApp()
	app.ctx = context.Background()
	app.config.Providers = []core.Provider{{ID: "p", BaseURL: server.URL, Model: "test-model", Mode: "json"}}
	app.connections["c"] = connection{server: core.Server{Name: "测试机", User: "deploy", Host: "private-host", KeyPath: "private-key-path"}}
	result, err := app.SuggestTerminalCommand("c", "p", "查看文件", "/tmp", "", "request", nil)
	if err != nil || result.Command != "ls -la" {
		t.Fatal(result, err)
	}
	if strings.Contains(received, "private-host") || strings.Contains(received, "private-key-path") {
		t.Fatal("unneeded connection data sent")
	}
	if len(app.ai) != 0 {
		t.Fatal("request leaked")
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
	if _, err = app.SuggestTerminalCommand("c", "p", "截图报错是什么原因", "/tmp", "docker ps\npermission denied", "image-request", []string{dataURL}); err != nil {
		t.Fatal(err)
	}
	messages := receivedRequest["messages"].([]any)
	parts := messages[1].(map[string]any)["content"].([]any)
	if len(parts) != 2 || !strings.Contains(parts[0].(map[string]any)["text"].(string), "permission denied") || parts[1].(map[string]any)["image_url"].(map[string]any)["url"] != dataURL {
		t.Fatal("referenced output or screenshot missing from multimodal request")
	}
	if _, err = app.SuggestTerminalCommand("c", "p", "查看", "/tmp", "", "bad-image", []string{"https://example.com/private.png"}); err == nil {
		t.Fatal("external image URL accepted")
	}
	if len(app.ai) != 0 {
		t.Fatal("image request leaked")
	}
	if _, err = app.SuggestTerminalCommand("missing", "p", "查看", "/tmp", "", "request", nil); err == nil {
		t.Fatal("closed connection accepted")
	}
}
