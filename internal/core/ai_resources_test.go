package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIAutomaticResourceContextAndValidation(t *testing.T) {
	for _, mode := range []string{"auto", "manual-server", "manual-asset", "server-denied", "asset-denied", "unknown-server", "unknown-asset", "duplicate-server", "duplicate-asset", "manual-duplicates", "missing-only", "empty-result"} {
		t.Run(mode, func(t *testing.T) {
			c, w := branchWorkflow()
			c.Servers[0].Name, c.Servers[1].Name = "测试服务器", "生产服务器"
			c.Servers[0].Host = "private-host-not-for-ai"
			c.Artifacts[0].Name = "前端产物"
			file := filepath.Join(t.TempDir(), "program")
			if err := os.WriteFile(file, []byte("binary"), 0600); err != nil {
				t.Fatal(err)
			}
			c.Artifacts[0].Files[0].Local = file
			c.Artifacts[0].Files[0].Remote = "/private-original-destination"
			req := AIRequest{Prompt: "将前端产物部署到测试服务器和生产服务器"}
			response := Draft{Workflow: &w}
			wantError := ""
			switch mode {
			case "manual-server":
				req.ServerIDs = []string{"a", "b"}
			case "manual-asset":
				req.ArtifactIDs = []string{"asset"}
			case "server-denied":
				req.ServerIDs = []string{"a"}
				wantError = "限定范围"
			case "asset-denied":
				c.Artifacts = append(c.Artifacts, ArtifactResource{ID: "other", Name: "其他产物"})
				req.ArtifactIDs = []string{"other"}
				wantError = "限定范围"
			case "unknown-server":
				w.Steps[0].Targets.Servers = []string{"invented"}
				wantError = "服务器不存在"
			case "unknown-asset":
				w.Steps[1].ArtifactID = "invented"
				wantError = "构建物不存在"
			case "duplicate-server":
				c.Servers[1].Name = c.Servers[0].Name
				wantError = "存在重名"
			case "duplicate-asset":
				c.Artifacts = append(c.Artifacts, ArtifactResource{ID: "duplicate", Name: c.Artifacts[0].Name})
				wantError = "存在重名"
			case "manual-duplicates":
				c.Servers[1].Name = c.Servers[0].Name
				c.Artifacts = append(c.Artifacts, ArtifactResource{ID: "duplicate", Name: c.Artifacts[0].Name})
				req.ServerIDs, req.ArtifactIDs = []string{"a", "b"}, []string{"asset"}
			case "missing-only":
				req.Prompt = "将不存在的产物部署到不存在的服务器"
				response = Draft{Summary: "请补充资源", Missing: []string{"找不到服务器：不存在的服务器", "找不到构建物：不存在的产物"}}
			case "empty-result":
				response = Draft{}
				wantError = "未生成"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				var payload struct {
					Messages []struct{ Content string } `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.Messages) != 2 {
					t.Error("invalid request", err)
					out.WriteHeader(400)
					return
				}
				var sent struct {
					Servers                              []struct{ ID, Name, Group string }
					Artifacts                            []ArtifactResource
					AutoMatchServers, AutoMatchArtifacts bool
				}
				contextJSON := payload.Messages[1].Content
				if err := json.Unmarshal([]byte(contextJSON), &sent); err != nil {
					t.Error(err)
				}
				if sent.AutoMatchServers != (len(req.ServerIDs) == 0) || sent.AutoMatchArtifacts != (len(req.ArtifactIDs) == 0) {
					t.Error("wrong matching mode")
				}
				wantServers, wantAssets := len(c.Servers), len(c.Artifacts)
				if len(req.ServerIDs) > 0 {
					wantServers = len(req.ServerIDs)
				}
				if len(req.ArtifactIDs) > 0 {
					wantAssets = len(req.ArtifactIDs)
				}
				if len(sent.Servers) != wantServers || len(sent.Artifacts) != wantAssets {
					t.Error("wrong resource candidates", sent)
				}
				if len(sent.Servers) == 0 || sent.Servers[0].Name != c.Servers[0].Name {
					t.Error("server names not provided for matching")
				}
				for _, asset := range sent.Artifacts {
					for _, f := range asset.Files {
						if f.Local != "" || f.Remote != "" {
							t.Error("saved asset paths leaked into model context")
						}
					}
				}
				for _, private := range []string{file, "private-host-not-for-ai", "/private-original-destination"} {
					if strings.Contains(contextJSON, private) {
						t.Errorf("private context leaked: %q", private)
					}
				}
				content, _ := json.Marshal(response)
				json.NewEncoder(out).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
			}))
			defer srv.Close()
			d, err := Generate(context.Background(), c, Provider{BaseURL: srv.URL, Mode: "json", Model: "test"}, "", req)
			if wantError != "" {
				if err == nil || !strings.Contains(err.Error(), wantError) {
					t.Fatalf("wanted %q, got %v", wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing-only" {
				if d.Workflow != nil || d.Combination != nil || len(d.Missing) != 2 {
					t.Fatal("missing resource explanation lost", d)
				}
			} else if d.Workflow == nil || len(d.Missing) != 0 || d.Workflow.Steps[1].ArtifactID != "asset" {
				t.Fatal("valid resource references lost", d)
			}
			if c.Artifacts[0].Files[0].Local != file {
				t.Fatal("saved asset was modified")
			}
		})
	}
}
