package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type FlowNode struct {
	ID  string  `json:"id"`
	Ref string  `json:"ref"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
}
type FlowEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type Flow struct {
	Nodes []FlowNode `json:"nodes"`
	Edges []FlowEdge `json:"edges"`
}

const StartRef = "$start"

func ValidateFlow(f Flow, refs map[string]bool) error {
	if len(f.Nodes) == 0 || len(f.Nodes) > 258 {
		return fmt.Errorf("流程图需要 1–258 个节点（含开始与服务器节点）")
	}
	return validateFlow(f, refs)
}

func validateFlow(f Flow, refs map[string]bool) error {
	nodes := map[string]bool{}
	parents := map[string][]string{}
	start := ""
	for _, n := range f.Nodes {
		if n.ID == "" || nodes[n.ID] || (!refs[n.Ref] && n.Ref != StartRef) {
			return fmt.Errorf("流程图节点为空、重复或引用不存在: %s", n.Ref)
		}
		nodes[n.ID] = true
		if n.Ref == StartRef {
			if start != "" {
				return fmt.Errorf("流程图只能有一个开始区块")
			}
			start = n.ID
		}
	}
	edges := map[FlowEdge]bool{}
	for _, e := range f.Edges {
		if !nodes[e.Source] || !nodes[e.Target] || edges[e] {
			return fmt.Errorf("流程图连线无效或重复")
		}
		edges[e] = true
		parents[e.Target] = append(parents[e.Target], e.Source)
		if e.Target == start {
			return fmt.Errorf("开始区块不能有前序连线")
		}
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("流程图不能有循环连线")
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, p := range parents[id] {
			if err := visit(p); err != nil {
				return err
			}
		}
		visiting[id] = false
		done[id] = true
		return nil
	}
	for id := range nodes {
		if err := visit(id); err != nil {
			return err
		}
	}
	if start != "" {
		reached := map[string]bool{start: true}
		var walk func(string)
		walk = func(id string) {
			for _, e := range f.Edges {
				if e.Source == id && !reached[e.Target] {
					reached[e.Target] = true
					walk(e.Target)
				}
			}
		}
		walk(start)
		if len(reached) != len(nodes) {
			return fmt.Errorf("所有区块都必须从开始区块连通")
		}
	}
	return nil
}

// Each node waits for all its predecessors; independent branches keep running on failure.
func RunFlow(ctx context.Context, f Flow, run func(FlowNode) error, blocked func(FlowNode)) error {
	refs := map[string]bool{}
	for _, n := range f.Nodes {
		refs[n.Ref] = true
	}
	if err := validateFlow(f, refs); err != nil {
		return err
	}
	indices := map[string]int{}
	parents := map[string][]string{}
	done := make([]chan struct{}, len(f.Nodes))
	errs := make([]error, len(f.Nodes))
	for i, n := range f.Nodes {
		indices[n.ID] = i
		done[i] = make(chan struct{})
	}
	for _, edge := range f.Edges {
		parents[edge.Target] = append(parents[edge.Target], edge.Source)
	}
	var wg sync.WaitGroup
	for i, n := range f.Nodes {
		wg.Add(1)
		go func(i int, n FlowNode) {
			defer wg.Done()
			defer close(done[i])
			failed := false
			for _, parent := range parents[n.ID] {
				p := indices[parent]
				<-done[p]
				if errs[p] != nil {
					failed = true
				}
			}
			if err := ctx.Err(); err != nil {
				errs[i] = err
				if n.Ref != StartRef {
					blocked(n)
				}
				return
			}
			if failed {
				errs[i] = fmt.Errorf("前序节点失败")
				blocked(n)
				return
			}
			if n.Ref != StartRef {
				errs[i] = run(n)
			}
		}(i, n)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (c Config) Reference(id string) (Node, bool) {
	if _, ok := c.Workflow(id); ok {
		return Node{WorkflowID: id}, true
	}
	for _, cmb := range c.Combinations {
		if cmb.ID == id {
			return cmb.Root, true
		}
	}
	return Node{}, false
}

func Ordered(f Flow, a, b string) bool {
	return Reaches(f, a, b) || Reaches(f, b, a)
}

func Reaches(f Flow, a, b string) bool {
	refs := map[string]string{}
	for _, n := range f.Nodes {
		refs[n.Ref] = n.ID
	}
	var reaches func(string, string, map[string]bool) bool
	reaches = func(from, to string, seen map[string]bool) bool {
		if from == to {
			return true
		}
		if seen[from] {
			return false
		}
		seen[from] = true
		for _, e := range f.Edges {
			if e.Source == from && reaches(e.Target, to, seen) {
				return true
			}
		}
		return false
	}
	from, haveA := refs[a]
	to, haveB := refs[b]
	return haveA && haveB && reaches(from, to, map[string]bool{})
}

func References(n Node) []string {
	result := []string{}
	if n.WorkflowID != "" {
		result = append(result, n.WorkflowID)
	}
	if n.Graph != nil {
		for _, v := range n.Graph.Nodes {
			if v.Ref != StartRef {
				result = append(result, v.Ref)
			}
		}
	}
	for _, child := range n.Children {
		result = append(result, References(child)...)
	}
	return result
}
