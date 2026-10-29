package core

import "fmt"

// NormalizeWorkflow moves legacy targets onto the canvas without changing existing dependencies.
func NormalizeWorkflow(w Workflow) Workflow {
	for _, st := range w.Steps {
		if st.Type == "server" {
			return w
		}
	}
	if len(w.Servers) == 0 {
		return w
	}
	flow := workflowFlow(w)
	used := map[string]bool{}
	for _, st := range w.Steps {
		used[st.ID] = true
	}
	for _, n := range flow.Nodes {
		used[n.ID] = true
	}
	id := "server-targets"
	for used[id] {
		id += "-legacy"
	}
	st := Step{ID: id, Name: "目标服务器", Type: "server", Targets: &Targets{Servers: w.Servers, Concurrency: w.Concurrency, Resources: w.Resources}}
	w.Steps = append([]Step{st}, w.Steps...)
	flow.Nodes = append([]FlowNode{{ID: id, Ref: id, X: 80, Y: 0}}, flow.Nodes...)
	flow.Edges = append([]FlowEdge(nil), flow.Edges...)
	start := ""
	for _, n := range flow.Nodes {
		if n.Ref == StartRef {
			start = n.ID
		}
	}
	if start != "" {
		for i := range flow.Edges {
			if flow.Edges[i].Source == start {
				flow.Edges[i].Source = id
			}
		}
		flow.Edges = append(flow.Edges, FlowEdge{Source: start, Target: id})
	}
	parents := map[string]bool{}
	for _, edge := range flow.Edges {
		parents[edge.Target] = true
	}
	for i := 1; i < len(flow.Nodes); i++ {
		if flow.Nodes[i].Ref == StartRef {
			continue
		}
		flow.Nodes[i].Y += 150
		if !parents[flow.Nodes[i].ID] {
			flow.Edges = append(flow.Edges, FlowEdge{Source: id, Target: flow.Nodes[i].ID})
		}
	}
	w.Flow = &flow
	w.Servers, w.Resources, w.Concurrency = []string{}, []string{}, 0
	return w
}

func workflowFlow(w Workflow) Flow {
	if w.Flow != nil {
		return *w.Flow
	}
	return SequentialFlow(w.Steps)
}

func WorkflowServers(w Workflow) []string {
	seen := map[string]bool{}
	result := []string{}
	add := func(ids []string) {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	add(w.Servers)
	for _, st := range w.Steps {
		if st.Type == "server" && st.Targets != nil {
			add(st.Targets.Servers)
		}
	}
	return result
}

// A remote step inherits one server node; crossing server contexts needs an explicit server node.
func bindWorkflow(w Workflow) (map[string]Step, error) {
	f := workflowFlow(w)
	steps, refs := map[string]Step{}, map[string]bool{}
	for _, st := range w.Steps {
		steps[st.ID], refs[st.ID] = st, true
	}
	if err := ValidateFlow(f, refs); err != nil {
		return nil, err
	}
	nodes, parents := map[string]FlowNode{}, map[string][]string{}
	for _, n := range f.Nodes {
		nodes[n.ID] = n
	}
	for _, edge := range f.Edges {
		parents[edge.Target] = append(parents[edge.Target], edge.Source)
	}
	bindings := map[string]Step{}
	var visit func(string) (Step, error)
	visit = func(id string) (Step, error) {
		if nodes[id].Ref == StartRef {
			return Step{}, nil
		}
		st := steps[nodes[id].Ref]
		if owner, ok := bindings[st.ID]; ok {
			return owner, nil
		}
		owner := st
		if st.Type != "server" {
			owner = Step{}
			ambiguous := false
			for _, parent := range parents[id] {
				p, err := visit(parent)
				if err != nil {
					return Step{}, err
				}
				if p.ID == "" {
					continue
				}
				if owner.ID != "" && owner.ID != p.ID {
					if st.Type != "workflow" {
						return Step{}, fmt.Errorf("%s: 前序来自不同服务器节点，请添加服务器节点明确执行目标", st.Name)
					}
					ambiguous = true
				}
				owner = p
			}
			if ambiguous {
				owner = Step{}
			}
			if owner.ID == "" && st.Type != "workflow" {
				return Step{}, fmt.Errorf("%s: 请连接前序服务器节点", st.Name)
			}
		}
		bindings[st.ID] = owner
		return owner, nil
	}
	for _, n := range f.Nodes {
		if _, err := visit(n.ID); err != nil {
			return nil, err
		}
	}
	return bindings, nil
}

func targetsServer(owner Step, id string) bool {
	if owner.Targets != nil {
		for _, sid := range owner.Targets.Servers {
			if sid == id {
				return true
			}
		}
	}
	return false
}

func sharedTargets(c Config, a, b Step) bool {
	if a.Targets == nil || b.Targets == nil {
		return false
	}
	for _, x := range a.Targets.Servers {
		sa, _ := c.Server(x)
		for _, y := range b.Targets.Servers {
			sb, _ := c.Server(y)
			if MachineKey(sa) == MachineKey(sb) {
				return true
			}
		}
	}
	return false
}
