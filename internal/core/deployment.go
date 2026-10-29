package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type deploymentTask struct {
	step, owner Step
	server      Server
}

type deploymentPlan struct {
	flow   Flow
	tasks  map[string]deploymentTask
	leases []lease
}

func planDeployment(c Config, w Workflow) (deploymentPlan, error) {
	p := deploymentPlan{tasks: map[string]deploymentTask{}}
	bindings, err := bindWorkflow(w)
	if err != nil {
		return p, err
	}
	f := workflowFlow(w)
	steps := map[string]Step{}
	for _, st := range w.Steps {
		steps[st.ID] = st
	}
	copies := map[string][]string{}
	locks := map[string]lease{}
	for i, n := range f.Nodes {
		if n.Ref == StartRef {
			id := fmt.Sprintf("start:%d", i)
			p.flow.Nodes = append(p.flow.Nodes, FlowNode{ID: id, Ref: StartRef})
			copies[n.ID] = []string{id}
			continue
		}
		st, owner := steps[n.Ref], bindings[n.Ref]
		if st.Type == "workflow" {
			id := fmt.Sprintf("call:%d", i)
			p.tasks[id] = deploymentTask{step: st, owner: owner}
			p.flow.Nodes = append(p.flow.Nodes, FlowNode{ID: id, Ref: id})
			copies[n.ID] = []string{id}
			continue
		}
		if owner.Targets == nil || owner.Targets.Concurrency < 1 {
			return p, fmt.Errorf("%s: 服务器节点配置无效", st.Name)
		}
		servers := owner.Targets.Servers
		for _, sid := range servers {
			id := fmt.Sprintf("%d:%s", i, sid)
			s, _ := c.Server(sid)
			p.tasks[id] = deploymentTask{st, owner, s}
			p.flow.Nodes = append(p.flow.Nodes, FlowNode{ID: id, Ref: id})
			copies[n.ID] = append(copies[n.ID], id)
			key := MachineKey(s)
			l := locks[key]
			l.machine, l.limit = key, HostLimit(c, s)
			if st.Type == "server" {
				l.resources = append(l.resources, owner.Targets.Resources...)
			}
			if st.Type == "upload" {
				l.paths = append(l.paths, st.LockPaths...)
				for _, file := range st.Files {
					l.paths = append(l.paths, uploadPath(file))
				}
			}
			locks[key] = l
		}
	}
	edges := map[FlowEdge]bool{}
	add := func(a, b string) {
		edge := FlowEdge{Source: a, Target: b}
		if !edges[edge] {
			p.flow.Edges = append(p.flow.Edges, edge)
			edges[edge] = true
		}
	}
	for _, edge := range f.Edges {
		for _, a := range copies[edge.Source] {
			for _, b := range copies[edge.Target] {
				x, y := p.tasks[a], p.tasks[b]
				if x.owner.ID == y.owner.ID && x.owner.ID != "" && x.server.ID != "" && y.server.ID != "" && x.server.ID != y.server.ID {
					continue
				}
				add(a, b)
			}
		}
	}
	// Serial targets run in selection order and wait for every step on the preceding host.
	for _, n := range p.flow.Nodes {
		task := p.tasks[n.ID]
		if task.step.Type != "server" || task.owner.Targets.Concurrency != 1 {
			continue
		}
		ids := task.owner.Targets.Servers
		for i := 1; i < len(ids); i++ {
			if task.server.ID != ids[i] {
				continue
			}
			for _, prev := range p.flow.Nodes {
				t := p.tasks[prev.ID]
				if t.owner.ID == task.owner.ID && t.server.ID == ids[i-1] {
					add(prev.ID, n.ID)
				}
			}
		}
	}
	for _, l := range locks {
		p.leases = append(p.leases, l)
	}
	return p, nil
}

func (e *Engine) workflow(ctx context.Context, c Config, w Workflow, id string) error {
	return e.runWorkflow(ctx, c, w, id, false)
}
func (e *Engine) runWorkflow(ctx context.Context, c Config, w Workflow, id string, reserved bool) error {
	ctx, closeSessions := e.withSessions(ctx)
	defer closeSessions()
	w = NormalizeWorkflow(w)
	var err error
	w, err = ResolveUploads(w, c)
	if err != nil {
		return err
	}
	p, err := planDeployment(c, w)
	if err != nil {
		return err
	}
	e.emit(Event{RunID: id, Workflow: w.ID, Status: "queued", Message: "等待执行资源"})
	// Locks span the whole deployment, including commands after upload. SSH concurrency is enforced separately.
	if !reserved {
		leases, err := workflowLeases(c, w, map[string]bool{})
		if err != nil {
			return err
		}
		release, err := e.gate.acquire(ctx, leases, 0)
		if err != nil {
			return err
		}
		defer release()
	}
	type progress struct {
		left    int
		started bool
	}
	var mu sync.Mutex
	groups := map[[2]string]*progress{}
	sems := map[string]chan struct{}{}
	for _, task := range p.tasks {
		if task.step.Type == "workflow" {
			continue
		}
		key := [2]string{task.owner.ID, task.server.ID}
		if groups[key] == nil {
			groups[key] = &progress{}
		}
		groups[key].left++
		if sems[task.owner.ID] == nil {
			sems[task.owner.ID] = make(chan struct{}, task.owner.Targets.Concurrency)
		}
	}
	complete := func(task deploymentTask) {
		if task.step.Type == "workflow" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		g := groups[[2]string{task.owner.ID, task.server.ID}]
		g.left--
		if g.left == 0 && g.started {
			<-sems[task.owner.ID]
		}
	}
	report := func(task deploymentTask, status, message, file string, current, total int64) {
		e.emit(Event{RunID: id, Workflow: w.ID, Server: task.server.ID, Step: task.step.ID, Status: status, Message: message, File: file, Current: current, Total: total})
	}
	return RunFlow(ctx, p.flow, func(n FlowNode) error {
		task := p.tasks[n.ID]
		defer complete(task)
		if task.step.Type == "workflow" {
			report(task, "running", "运行引用的工作流", "", 0, 0)
			err := e.nodeReserved(ctx, c, Node{WorkflowID: task.step.WorkflowID}, id, true)
			status := "success"
			if err != nil {
				status = "failed"
				var unknown *UnknownError
				if errors.As(err, &unknown) {
					status = "unknown"
				} else if ctx.Err() != nil {
					status = "cancelled"
				}
				report(task, status, err.Error(), "", 0, 0)
			} else {
				report(task, status, "引用工作流已完成", "", 0, 0)
			}
			return err
		}
		if task.step.Type == "server" {
			select {
			case sems[task.owner.ID] <- struct{}{}:
				mu.Lock()
				groups[[2]string{task.owner.ID, task.server.ID}].started = true
				mu.Unlock()
			case <-ctx.Done():
				return ctx.Err()
			}
			report(task, "success", "已选择执行目标", "", 0, 0)
			return nil
		}
		release, err := e.stepGate.Acquire(ctx, MachineKey(task.server), HostLimit(c, task.server), c.Concurrency, nil, nil)
		if err != nil {
			return err
		}
		defer release()
		report(task, "running", task.step.Name, "", 0, 0)
		task.step.UploadProtocol = c.UploadProtocol
		err = e.step(ctx, task.server, task.step, func(state, msg, file string, current, total int64) { report(task, state, msg, file, current, total) })
		if err != nil {
			status := "failed"
			var unknown *UnknownError
			if errors.As(err, &unknown) {
				status = "unknown"
			} else if ctx.Err() != nil {
				status = "cancelled"
			}
			report(task, status, err.Error(), "", 0, 0)
		} else {
			report(task, "success", "步骤完成", "", 0, 0)
		}
		return err
	}, func(n FlowNode) {
		task := p.tasks[n.ID]
		complete(task)
		report(task, "blocked", "前序节点失败或运行已取消", "", 0, 0)
	})
}
