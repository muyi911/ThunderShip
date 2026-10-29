package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Session interface {
	Run(context.Context, Step, func(string)) error
	Publish(context.Context, []Artifact, Step, func(string, int64, int64, string)) error
	CheckFiles([]Artifact) error
	Close()
}
type Connector func(context.Context, Server) (Session, error)
type Event struct {
	Seq      int    `json:"seq"`
	Time     string `json:"time"`
	RunID    string `json:"runId"`
	Workflow string `json:"workflow"`
	Server   string `json:"server"`
	Step     string `json:"step"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	File     string `json:"file"`
	Current  int64  `json:"current"`
	Total    int64  `json:"total"`
}
type Run struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Started  string `json:"started"`
	Finished string `json:"finished"`
	Error    string `json:"error"`
	Root     Node   `json:"root"`
}
type lease struct {
	machine          string
	paths, resources []string
	limit            int
}
type Gate struct {
	mu     sync.Mutex
	active map[string]lease
	wake   chan struct{}
}

func NewGate() *Gate { return &Gate{active: map[string]lease{}, wake: make(chan struct{})} }
func (g *Gate) Acquire(ctx context.Context, machine string, limit, global int, paths, resources []string) (func(), error) {
	return g.acquire(ctx, []lease{{machine: machine, limit: limit, paths: paths, resources: resources}}, global)
}

// Reserve every host together so workflows switching hosts cannot deadlock while holding another host's lock.
func (g *Gate) acquire(ctx context.Context, requested []lease, global int) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		g.mu.Lock()
		counts := map[string]int{}
		conflict := global > 0 && len(g.active)+len(requested) > global
		for _, want := range requested {
			for _, l := range g.active {
				if l.machine != want.machine {
					continue
				}
				counts[want.machine]++
				for _, a := range l.paths {
					for _, b := range want.paths {
						if Overlap(a, b) {
							conflict = true
						}
					}
				}
				for _, a := range l.resources {
					for _, b := range want.resources {
						if a == b {
							conflict = true
						}
					}
				}
			}
			if want.limit > 0 && counts[want.machine] >= want.limit {
				conflict = true
			}
		}
		if !conflict {
			keys := []string{}
			for _, want := range requested {
				key := ID()
				keys = append(keys, key)
				g.active[key] = want
			}
			g.mu.Unlock()
			return func() {
				g.mu.Lock()
				for _, key := range keys {
					delete(g.active, key)
				}
				close(g.wake)
				g.wake = make(chan struct{})
				g.mu.Unlock()
			}, nil
		}
		wake := g.wake
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wake:
		}
	}
}

type Engine struct {
	mu       sync.Mutex
	dir      string
	gate     *Gate
	stepGate *Gate
	connect  Connector
	runs     map[string]*Run
	events   map[string][]Event
	cancels  map[string]context.CancelFunc
	OnEvent  func(Event)
}

func NewEngine(dir string, connect Connector) (*Engine, error) {
	e := &Engine{dir: dir, gate: NewGate(), stepGate: NewGate(), connect: connect, runs: map[string]*Run{}, events: map[string][]Event{}, cancels: map[string]context.CancelFunc{}}
	entries, err := os.ReadDir(filepath.Join(dir, "runs"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "runs", d.Name(), "run.json"))
		if err != nil {
			continue
		}
		var r Run
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		if r.Status == "running" || r.Status == "preparing" {
			r.Status = "unknown"
			r.Error = "应用关闭时任务未完成，请核查远端状态后再运行"
			if err = WriteJSON(filepath.Join(dir, "runs", d.Name(), "run.json"), r); err != nil {
				return nil, err
			}
		}
		e.runs[r.ID] = &r
	}
	return e, nil
}
func (e *Engine) List() []Run {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := []Run{}
	for _, x := range e.runs {
		r = append(r, *x)
	}
	return r
}
func (e *Engine) Events(id string, after int) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	all, ok := e.events[id]
	if !ok {
		b, _ := os.ReadFile(filepath.Join(e.dir, "runs", id, "events.jsonl"))
		for _, line := range splitLines(b) {
			var ev Event
			if json.Unmarshal(line, &ev) == nil {
				all = append(all, ev)
			}
		}
		e.events[id] = all
	}
	out := []Event{}
	for _, v := range all {
		if v.Seq > after {
			out = append(out, v)
		}
	}
	return out
}
func splitLines(b []byte) [][]byte {
	var r [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			r = append(r, b[start:i])
			start = i + 1
		}
	}
	return r
}
func (e *Engine) emit(ev Event) {
	e.mu.Lock()
	ev.Seq = len(e.events[ev.RunID]) + 1
	ev.Time = time.Now().Format(time.RFC3339)
	e.events[ev.RunID] = append(e.events[ev.RunID], ev)
	b, _ := json.Marshal(ev)
	f, err := os.OpenFile(filepath.Join(e.dir, "runs", ev.RunID, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		_, err = f.Write(append(b, '\n'))
		f.Close()
	}
	if err != nil {
		if cancel := e.cancels[ev.RunID]; cancel != nil {
			cancel()
		}
		if r := e.runs[ev.RunID]; r != nil {
			r.Error = "运行日志写入失败: " + err.Error()
		}
	}
	callback := e.OnEvent
	e.mu.Unlock()
	if callback != nil {
		callback(ev)
	}
}
func (e *Engine) Cancel(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c := e.cancels[id]; c != nil {
		c()
	}
}
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.cancels {
		c()
	}
}
func (e *Engine) LockTransfer(ctx context.Context, c Config, s Server, mappings []FileMapping) (func(), error) {
	paths := []string{}
	for _, f := range mappings {
		paths = append(paths, f.Remote)
	}
	limit := HostLimit(c, s)
	release, err := e.gate.Acquire(ctx, MachineKey(s), limit, 0, paths, nil)
	if err != nil {
		return nil, err
	}
	releaseStep, err := e.stepGate.Acquire(ctx, MachineKey(s), limit, c.Concurrency, nil, nil)
	if err != nil {
		release()
		return nil, err
	}
	return func() { releaseStep(); release() }, nil
}
func Selected(c Config, n Node) ([]Workflow, error) {
	return selected(c, n, true)
}
func selected(c Config, n Node, checkLocal bool) ([]Workflow, error) {
	if err := ValidateNode(n, c); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var all []Workflow
	var visit func(Node) error
	visit = func(n Node) error {
		if n.WorkflowID != "" && !seen[n.WorkflowID] {
			w, leaf := c.Workflow(n.WorkflowID)
			if !leaf {
				ref, _ := c.Reference(n.WorkflowID)
				return visit(ref)
			}
			errs := ValidateWorkflow(w, c, checkLocal)
			if len(errs) > 0 {
				return fmt.Errorf("%s: %v", w.Name, errs)
			}
			seen[w.ID] = true
			all = append(all, w)
			for _, st := range w.Steps {
				if st.Type == "workflow" {
					if err := visit(Node{WorkflowID: st.WorkflowID}); err != nil {
						return err
					}
				}
			}
		}
		if n.Graph != nil {
			for _, v := range n.Graph.Nodes {
				if v.Ref == StartRef {
					continue
				}
				if err := visit(Node{WorkflowID: v.Ref}); err != nil {
					return err
				}
			}
		}
		for _, x := range n.Children {
			if err := visit(x); err != nil {
				return err
			}
		}
		return nil
	}
	err := visit(n)
	return all, err
}
func (e *Engine) Preflight(ctx context.Context, c Config, n Node) []string {
	ctx, closeSessions := e.withSessions(ctx)
	defer closeSessions()
	sessions := ctx.Value(sessionScopeKey{}).(*sessionScope)
	ws, err := Selected(c, n)
	if err != nil {
		return []string{err.Error()}
	}
	result := []string{}
	for _, w := range ws {
		w = NormalizeWorkflow(w)
		w, _ = ResolveUploads(w, c)
		bindings, _ := bindWorkflow(w)
		for _, sid := range WorkflowServers(w) {
			s, _ := c.Server(sid)
			_, err := sessions.get(ctx, s)
			if err != nil {
				result = append(result, s.Name+": "+err.Error())
				continue
			}
			for _, st := range w.Steps {
				if st.Type != "upload" || !targetsServer(bindings[st.ID], sid) {
					continue
				}
				files, err := Expand(st.Files)
				if err == nil {
					target := s
					target.SudoUpload = s.SudoUpload || st.Sudo
					var transfer Session
					var release func()
					transfer, release, err = sessions.transfer(ctx, target)
					if err == nil {
						err = transfer.CheckFiles(files)
						release()
					}
				}
				if err != nil {
					result = append(result, s.Name+" / "+st.Name+": "+err.Error())
				}
			}
		}
	}
	return result
}
func (e *Engine) Start(parent context.Context, c Config, n Node, name string) (string, error) {
	if _, err := Selected(c, n); err != nil {
		return "", err
	}
	if c.Concurrency < 1 || c.Concurrency > 64 {
		return "", fmt.Errorf("全局并发数须为 1–64")
	}
	id := ID()
	dir := filepath.Join(e.dir, "runs", id)
	frozen, err := Freeze(c, n, dir)
	if err != nil {
		return "", err
	}
	if err = WriteJSON(filepath.Join(dir, "snapshot.json"), frozen); err != nil {
		return "", err
	}
	r := &Run{ID: id, Name: name, Status: "running", Started: time.Now().Format(time.RFC3339), Root: n}
	if err = WriteJSON(filepath.Join(dir, "run.json"), r); err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(parent)
	e.mu.Lock()
	e.runs[id] = r
	e.cancels[id] = cancel
	e.events[id] = []Event{}
	e.mu.Unlock()
	go func() {
		defer cancel()
		err := e.node(ctx, frozen, n, id)
		e.mu.Lock()
		r.Status = "success"
		if err != nil {
			r.Status = "failed"
			r.Error = err.Error()
			var unknown *UnknownError
			if errors.As(err, &unknown) {
				r.Status = "unknown"
			} else if ctx.Err() != nil {
				r.Status = "cancelled"
			}
		}
		if r.Error != "" && r.Status == "success" {
			r.Status = "failed"
		}
		r.Finished = time.Now().Format(time.RFC3339)
		if saveErr := WriteJSON(filepath.Join(dir, "run.json"), r); saveErr != nil {
			r.Status = "unknown"
			r.Error = "结果保存失败: " + saveErr.Error()
		}
		status, msg := r.Status, r.Error
		delete(e.cancels, id)
		e.mu.Unlock()
		e.emit(Event{RunID: id, Status: status, Message: msg})
	}()
	return id, nil
}
func (e *Engine) node(ctx context.Context, c Config, n Node, id string) error {
	ctx, closeSessions := e.withSessions(ctx)
	defer closeSessions()
	return e.nodeReserved(ctx, c, n, id, false)
}
func (e *Engine) nodeReserved(ctx context.Context, c Config, n Node, id string, reserved bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n.WorkflowID != "" {
		w, leaf := c.Workflow(n.WorkflowID)
		if !leaf {
			ref, _ := c.Reference(n.WorkflowID)
			return e.nodeReserved(ctx, c, ref, id, reserved)
		}
		return e.runWorkflow(ctx, c, w, id, reserved)
	}
	localGate := NewGate()
	runChild := func(ch Node) error {
		if reserved {
			ls, err := nodeLeases(c, ch, map[string]bool{})
			if err != nil {
				return err
			}
			release, err := localGate.acquire(ctx, ls, 0)
			if err != nil {
				return err
			}
			defer release()
		}
		return e.nodeReserved(ctx, c, ch, id, reserved)
	}
	if n.Graph != nil {
		return RunFlow(ctx, *n.Graph, func(v FlowNode) error { return runChild(Node{WorkflowID: v.Ref}) }, func(v FlowNode) { e.block(Node{WorkflowID: v.Ref}, id, "前序节点失败或运行已取消") })
	}
	if n.Mode == "serial" {
		for i, ch := range n.Children {
			if err := runChild(ch); err != nil {
				for _, blocked := range n.Children[i+1:] {
					e.block(blocked, id, "前序任务失败，未启动")
				}
				return err
			}
		}
		return nil
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, ch := range n.Children {
		wg.Add(1)
		go func(ch Node) {
			defer wg.Done()
			if err := runChild(ch); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(ch)
	}
	wg.Wait()
	return errors.Join(errs...)
}
func (e *Engine) block(n Node, id, msg string) {
	if n.WorkflowID != "" {
		e.emit(Event{RunID: id, Workflow: n.WorkflowID, Status: "blocked", Message: msg})
	}
	for _, ch := range n.Children {
		e.block(ch, id, msg)
	}
}
