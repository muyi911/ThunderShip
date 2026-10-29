package core

import "fmt"

// A caller reserves its complete subtree once; a child must never reacquire its parent's locks.
func workflowLeases(c Config, w Workflow, visiting map[string]bool) ([]lease, error) {
	if visiting[w.ID] || len(visiting) > 16 {
		return nil, fmt.Errorf("工作流引用存在循环或嵌套过深")
	}
	visiting[w.ID] = true
	defer delete(visiting, w.ID)
	w = NormalizeWorkflow(w)
	w, err := ResolveUploads(w, c)
	if err != nil {
		return nil, err
	}
	p, err := planDeployment(c, w)
	if err != nil {
		return nil, err
	}
	all := p.leases
	for _, st := range w.Steps {
		if st.Type == "workflow" {
			ls, err := referenceLeases(c, st.WorkflowID, visiting)
			if err != nil {
				return nil, err
			}
			all = append(all, ls...)
		}
	}
	return mergeLeases(all), nil
}

func referenceLeases(c Config, id string, visiting map[string]bool) ([]lease, error) {
	if w, ok := c.Workflow(id); ok {
		return workflowLeases(c, w, visiting)
	}
	if visiting[id] || len(visiting) > 16 {
		return nil, fmt.Errorf("工作流引用存在循环或嵌套过深")
	}
	n, ok := c.Reference(id)
	if !ok {
		return nil, fmt.Errorf("工作流不存在: %s", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	return nodeLeases(c, n, visiting)
}

func nodeLeases(c Config, n Node, visiting map[string]bool) ([]lease, error) {
	all := []lease{}
	for _, ref := range References(n) {
		ls, err := referenceLeases(c, ref, visiting)
		if err != nil {
			return nil, err
		}
		all = append(all, ls...)
	}
	return mergeLeases(all), nil
}

func mergeLeases(all []lease) []lease {
	byHost := map[string]lease{}
	for _, l := range all {
		old, exists := byHost[l.machine]
		if !exists {
			byHost[l.machine] = l
			continue
		}
		old.paths = append(old.paths, l.paths...)
		old.resources = append(old.resources, l.resources...)
		byHost[l.machine] = old
	}
	result := []lease{}
	for _, l := range byHost {
		result = append(result, l)
	}
	return result
}

func validateCallConflicts(c Config, w Workflow, bindings map[string]Step) error {
	hasCall := false
	for _, st := range w.Steps {
		hasCall = hasCall || st.Type == "workflow"
	}
	if !hasCall {
		return nil
	}
	f := workflowFlow(w)
	// shortcut: host slots span one server context; split contexts after a global call until scheduling supports phases.
	for _, call := range w.Steps {
		if call.Type != "workflow" {
			continue
		}
		for _, st := range w.Steps {
			owner := bindings[st.ID]
			if st.Type == "server" || st.Type == "workflow" || owner.Targets == nil || len(owner.Targets.Servers) <= owner.Targets.Concurrency {
				continue
			}
			if Reaches(f, owner.ID, call.ID) && Reaches(f, call.ID, st.ID) {
				return fmt.Errorf("%s: 分批执行服务器时，请在运行工作流之后添加新的服务器区块，再连接 %s，避免前后阶段互相等待", call.Name, st.Name)
			}
		}
	}
	footprints := map[string][]lease{}
	for _, st := range w.Steps {
		if st.Type == "workflow" {
			ls, err := referenceLeases(c, st.WorkflowID, map[string]bool{w.ID: true})
			if err != nil {
				return err
			}
			footprints[st.ID] = ls
			continue
		}
		owner := bindings[st.ID]
		if owner.Targets == nil || st.Type == "server" {
			continue
		}
		for _, sid := range owner.Targets.Servers {
			s, _ := c.Server(sid)
			l := lease{machine: MachineKey(s), resources: owner.Targets.Resources}
			if st.Type == "upload" {
				for _, f := range st.Files {
					l.paths = append(l.paths, f.Remote)
				}
			}
			footprints[st.ID] = append(footprints[st.ID], l)
		}
	}
	for i, a := range w.Steps {
		for _, b := range w.Steps[i+1:] {
			if (a.Type != "workflow" && b.Type != "workflow") || Ordered(f, a.ID, b.ID) {
				continue
			}
			for _, x := range footprints[a.ID] {
				for _, y := range footprints[b.ID] {
					if x.machine != y.machine {
						continue
					}
					conflict := false
					for _, p := range x.paths {
						for _, q := range y.paths {
							conflict = conflict || Overlap(p, q)
						}
					}
					for _, p := range x.resources {
						for _, q := range y.resources {
							conflict = conflict || p == q
						}
					}
					if conflict {
						return fmt.Errorf("%s 与 %s 涉及相同上传路径或资源锁，请连线明确先后顺序", a.Name, b.Name)
					}
				}
			}
		}
	}
	return nil
}
