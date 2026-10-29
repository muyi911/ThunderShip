package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

func ID() string {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

type Server struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Auth        string `json:"auth"`
	KeyPath     string `json:"keyPath"`
	HasSecret   bool   `json:"hasSecret"`
	Fingerprint string `json:"fingerprint"`
	Machine     string `json:"machine"`
	Concurrency int    `json:"concurrency"`
	SudoUpload  bool   `json:"sudoUpload"`
	SudoUser    string `json:"sudoUser"`
	SFTPPath    string `json:"sftpPath"`
}
type FileMapping struct {
	ID            string `json:"id"`
	Source        string `json:"source,omitempty"`
	Name          string `json:"name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Local         string `json:"local"`
	Remote        string `json:"remote"`
	Executable    bool   `json:"executable"`
	IncludeFolder bool   `json:"includeFolder,omitempty"`
}
type Step struct {
	UploadProtocol string        `json:"-"`
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Type           string        `json:"type"`
	Targets        *Targets      `json:"targets,omitempty"`
	ArtifactID     string        `json:"artifactId,omitempty"`
	WorkflowID     string        `json:"workflowId,omitempty"`
	Files          []FileMapping `json:"files"`
	Command        string        `json:"command"`
	CommandMode    string        `json:"commandMode,omitempty"`
	Commands       []string      `json:"commands,omitempty"`
	Directory      string        `json:"directory"`
	Sudo           bool          `json:"sudo"`
	Timeout        int           `json:"timeout"`
	Interval       int           `json:"interval"`
	BeforePublish  string        `json:"beforePublish"`
	LockPaths      []string      `json:"-"`
}
type Targets struct {
	Servers     []string `json:"servers"`
	Concurrency int      `json:"concurrency"`
	Resources   []string `json:"resources"`
}
type Workflow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     int      `json:"version"`
	Servers     []string `json:"servers"`
	Concurrency int      `json:"concurrency"`
	Resources   []string `json:"resources"`
	Steps       []Step   `json:"steps"`
	Flow        *Flow    `json:"flow,omitempty"`
}
type Node struct {
	Graph      *Flow  `json:"graph,omitempty"`
	Mode       string `json:"mode"`
	WorkflowID string `json:"workflowId"`
	Children   []Node `json:"children"`
}
type Combination struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root Node   `json:"root"`
}
type Provider struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	BaseURL   string   `json:"baseUrl"`
	Model     string   `json:"model"`
	Mode      string   `json:"mode"`
	AllowHTTP bool     `json:"allowHttp"`
	HasSecret bool     `json:"hasSecret"`
	Models    []string `json:"models"`
}
type Config struct {
	Tunnels        map[string][]TunnelRule `json:"tunnels,omitempty"`
	ResourceOrder  map[string][]string     `json:"resourceOrder,omitempty"`
	Artifacts      []ArtifactResource      `json:"artifacts"`
	Servers        []Server                `json:"servers"`
	Workflows      []Workflow              `json:"workflows"`
	Combinations   []Combination           `json:"combinations"`
	Providers      []Provider              `json:"providers"`
	Concurrency    int                     `json:"concurrency"`
	Theme          string                  `json:"theme"`
	UploadProtocol string                  `json:"uploadProtocol"`
}

func EmptyConfig() Config {
	return Config{Artifacts: []ArtifactResource{}, Servers: []Server{}, Workflows: []Workflow{}, Combinations: []Combination{}, Providers: []Provider{}, Concurrency: 4, Theme: "system", UploadProtocol: "sftp"}
}
func (c Config) Workflow(id string) (Workflow, bool) {
	for _, w := range c.Workflows {
		if w.ID == id {
			return w, true
		}
	}
	return Workflow{}, false
}
func (c Config) Server(id string) (Server, bool) {
	for _, s := range c.Servers {
		if s.ID == id {
			return s, true
		}
	}
	return Server{}, false
}
func ValidateWorkflow(w Workflow, c Config, checkLocal bool) []string {
	w = NormalizeWorkflow(w)
	var errs []string
	resolved, resolveErr := ResolveUploads(w, c)
	if resolveErr != nil {
		errs = append(errs, resolveErr.Error())
	} else {
		w = resolved
	}
	if strings.TrimSpace(w.Name) == "" {
		errs = append(errs, "请填写工作流名称")
	}
	if len(w.Servers) > 0 || len(w.Resources) > 0 {
		errs = append(errs, "请将目标服务器和资源锁配置在服务器节点中")
	}
	if len(w.Steps) == 0 {
		errs = append(errs, "请添加至少一个步骤")
	}
	stepIDs := map[string]bool{}
	for _, s := range w.Steps {
		if s.ID == "" || stepIDs[s.ID] {
			errs = append(errs, "步骤 ID 不能为空或重复")
		}
		stepIDs[s.ID] = true
	}
	bindings, bindErr := bindWorkflow(w)
	if bindErr != nil {
		errs = append(errs, bindErr.Error())
	}
	if w.Flow != nil {
		if e := ValidateFlow(*w.Flow, stepIDs); e != nil {
			errs = append(errs, e.Error())
		}
		used := map[string]bool{}
		for _, n := range w.Flow.Nodes {
			if n.Ref == StartRef {
				continue
			}
			if used[n.Ref] {
				errs = append(errs, "一个步骤只能对应一个节点")
			}
			used[n.Ref] = true
		}
		if len(used) != len(stepIDs) {
			errs = append(errs, "所有步骤都必须在流程图中")
		}
		for i, a := range w.Steps {
			for _, b := range w.Steps[i+1:] {
				if a.Type != "upload" || b.Type != "upload" || Ordered(*w.Flow, a.ID, b.ID) || bindErr != nil || !sharedTargets(c, bindings[a.ID], bindings[b.ID]) {
					continue
				}
				for _, af := range a.Files {
					for _, bf := range b.Files {
						if Overlap(uploadPath(af), uploadPath(bf)) {
							errs = append(errs, "上传路径重叠的节点必须通过连线明确先后顺序")
						}
					}
				}
			}
		}
	}
	for _, s := range w.Steps {
		if s.Type == "workflow" {
			if s.WorkflowID == "" {
				errs = append(errs, s.Name+": 请选择工作流")
			} else if err := validateNode(Node{WorkflowID: s.WorkflowID}, c, 0, map[string]bool{w.ID: true}); err != nil {
				errs = append(errs, s.Name+": "+err.Error())
			}
			if s.Targets != nil || s.ArtifactID != "" || len(s.Files) != 0 {
				errs = append(errs, s.Name+": 工作流组件仅引用已有工作流，不配置服务器或文件")
			}
			continue
		}
		if s.Type == "server" {
			if s.Targets == nil || len(s.Targets.Servers) == 0 {
				errs = append(errs, s.Name+": 请选择目标服务器")
				continue
			}
			if s.Targets.Concurrency < 1 || s.Targets.Concurrency > 64 {
				errs = append(errs, s.Name+": 服务器并发数须为 1–64")
			}
			seen := map[string]bool{}
			for _, id := range s.Targets.Servers {
				if _, ok := c.Server(id); !ok {
					errs = append(errs, "服务器引用不存在: "+id)
				}
				if seen[id] {
					errs = append(errs, "重复服务器: "+id)
				}
				seen[id] = true
			}
			continue
		}
		if s.Targets != nil {
			errs = append(errs, s.Name+": 仅服务器节点能配置执行目标")
		}
		if s.Timeout < 1 || s.Timeout > 86400 {
			errs = append(errs, s.Name+": 超时须为 1–86400 秒")
		}
		switch s.Type {
		case "upload":
			if len(s.Files) == 0 {
				errs = append(errs, s.Name+": 请添加文件")
			}
			for _, f := range s.Files {
				if (f.Local == "" && (checkLocal || s.ArtifactID == "")) || !validUploadPath(f) {
					errs = append(errs, s.Name+": 本地路径或远端绝对路径无效")
				}
			}
			if checkLocal {
				for _, f := range s.Files {
					if f.Kind != "" {
						if err := validateInput(f.Local, f.Kind); err != nil {
							errs = append(errs, s.Name+": "+err.Error())
						}
					}
				}
				if _, e := Expand(s.Files); e != nil {
					errs = append(errs, s.Name+": "+e.Error())
				}
			}
		case "command", "health":
			if s.CommandMode != "" && s.CommandMode != "script" && s.CommandMode != "lines" {
				errs = append(errs, s.Name+": 命令模式无效")
			}
			if s.CommandMode == "lines" {
				if s.Type != "command" {
					errs = append(errs, s.Name+": 健康检查仅支持脚本模式")
				}
				if len(s.Commands) < 1 || len(s.Commands) > 256 {
					errs = append(errs, s.Name+": 请添加 1–256 条命令")
				}
				for i, cmd := range s.Commands {
					if strings.TrimSpace(cmd) == "" || strings.ContainsAny(cmd, "\x00\r\n") {
						errs = append(errs, fmt.Sprintf("%s: 第 %d 条命令不能为空或包含换行；多行逻辑请使用脚本模式", s.Name, i+1))
					}
				}
			} else if strings.TrimSpace(s.Command) == "" {
				errs = append(errs, s.Name+": 命令不能为空")
			}
			if s.Directory != "" && !ValidRemote(s.Directory) {
				errs = append(errs, s.Name+": 工作目录须为绝对路径")
			}
			if s.Type == "health" && (s.Interval < 1 || s.Interval > 3600) {
				errs = append(errs, s.Name+": 检查间隔须为 1–3600 秒")
			}
		default:
			errs = append(errs, "不支持的步骤类型: "+s.Type)
		}
	}
	if len(errs) == 0 {
		if err := validateCallConflicts(c, w, bindings); err != nil {
			errs = append(errs, err.Error())
		}
	}
	return errs
}
func ValidRemote(p string) bool {
	if !path.IsAbs(p) || p == "/" || strings.ContainsAny(p, "\x00\r\n\\") {
		return false
	}
	for _, v := range strings.Split(p, "/") {
		if v == ".." {
			return false
		}
	}
	return path.Clean(p) == p
}
func ValidateNode(n Node, c Config) error { return validateNode(n, c, 0, map[string]bool{}) }
func validateNode(n Node, c Config, depth int, visiting map[string]bool) error {
	if depth > 16 {
		return fmt.Errorf("组合嵌套不能超过 16 层")
	}
	if n.WorkflowID != "" {
		if n.Mode != "" || len(n.Children) != 0 || n.Graph != nil {
			return fmt.Errorf("工作流引用不能包含分组")
		}
		ref, ok := c.Reference(n.WorkflowID)
		if !ok {
			return fmt.Errorf("工作流不存在: %s", n.WorkflowID)
		}
		if visiting[n.WorkflowID] {
			return fmt.Errorf("工作流引用存在循环")
		}
		visiting[n.WorkflowID] = true
		defer delete(visiting, n.WorkflowID)
		if w, leaf := c.Workflow(n.WorkflowID); leaf {
			for _, st := range w.Steps {
				if st.Type == "workflow" {
					if st.WorkflowID == "" {
						return fmt.Errorf("请选择引用的工作流")
					}
					if err := validateNode(Node{WorkflowID: st.WorkflowID}, c, depth+1, visiting); err != nil {
						return err
					}
				}
			}
		} else {
			err := validateNode(ref, c, depth+1, visiting)
			return err
		}
		return nil
	}
	if n.Graph != nil {
		if n.Mode != "" || len(n.Children) != 0 {
			return fmt.Errorf("流程图不能同时包含旧分组")
		}
		refs := map[string]bool{}
		for _, w := range c.Workflows {
			refs[w.ID] = true
		}
		for _, w := range c.Combinations {
			refs[w.ID] = true
		}
		if err := ValidateFlow(*n.Graph, refs); err != nil {
			return err
		}
		if len(n.Graph.Nodes) == 1 && n.Graph.Nodes[0].Ref == StartRef {
			return fmt.Errorf("请添加要运行的工作流")
		}
		for _, v := range n.Graph.Nodes {
			if v.Ref == StartRef {
				continue
			}
			if err := validateNode(Node{WorkflowID: v.Ref}, c, depth+1, visiting); err != nil {
				return err
			}
		}
		return nil
	}
	if n.Mode != "serial" && n.Mode != "parallel" {
		return fmt.Errorf("分组只能是串行或并行")
	}
	if len(n.Children) == 0 {
		return fmt.Errorf("分组不能为空")
	}
	for _, ch := range n.Children {
		if e := validateNode(ch, c, depth+1, visiting); e != nil {
			return e
		}
	}
	return nil
}
func MachineKey(s Server) string {
	if s.Machine != "" {
		return strings.ToLower(strings.TrimSpace(s.Machine))
	}
	return fmt.Sprintf("%s:%d", strings.ToLower(strings.TrimSpace(s.Host)), s.Port)
}
func HostLimit(c Config, s Server) int {
	limit := s.Concurrency
	if limit < 1 {
		limit = 0
	}
	for _, alias := range c.Servers {
		if MachineKey(alias) == MachineKey(s) && alias.Concurrency > 0 && (limit == 0 || alias.Concurrency < limit) {
			limit = alias.Concurrency
		}
	}
	return limit
}
func Overlap(a, b string) bool {
	a = path.Clean(a)
	b = path.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
