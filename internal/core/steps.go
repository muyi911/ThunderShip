package core

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func SequentialFlow(steps []Step) Flow {
	f := Flow{Nodes: []FlowNode{}, Edges: []FlowEdge{}}
	for i, st := range steps {
		f.Nodes = append(f.Nodes, FlowNode{ID: st.ID, Ref: st.ID, X: 80, Y: float64(70 + i*130)})
		if i > 0 {
			f.Edges = append(f.Edges, FlowEdge{Source: steps[i-1].ID, Target: st.ID})
		}
	}
	return f
}

func (e *Engine) step(ctx context.Context, s Server, st Step, report func(string, string, string, int64, int64)) error {
	ctx, closeSessions := e.withSessions(ctx)
	defer closeSessions()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(st.Timeout)*time.Second)
	defer cancel()
	if st.Type == "upload" && st.Sudo {
		s.SudoUpload = true
	}
	sessions := ctx.Value(sessionScopeKey{}).(*sessionScope)
	var r Session
	var err error
	if st.Type == "upload" {
		var release func()
		r, release, err = sessions.transfer(ctx, s)
		if err == nil {
			defer release()
		}
	} else {
		r, err = sessions.get(ctx, s)
	}
	if err != nil {
		return err
	}
	if st.Type == "upload" {
		files, err := Expand(st.Files)
		if err != nil {
			return err
		}
		return r.Publish(ctx, files, Step{Command: st.BeforePublish, Sudo: st.Sudo, UploadProtocol: st.UploadProtocol}, func(file string, current, total int64, state string) {
			if state == "log" {
				report(state, file, "", 0, 0)
			} else {
				report(state, "", file, current, total)
			}
		})
	}
	if st.Type == "command" && st.CommandMode == "lines" {
		for i, cmd := range st.Commands {
			if err := ctx.Err(); err != nil {
				return err
			}
			report("log", fmt.Sprintf("命令 %d/%d · 执行中", i+1, len(st.Commands)), "", 0, 0)
			line := st
			line.Command = cmd
			if err := r.Run(ctx, line, func(msg string) { report("log", msg, "", 0, 0) }); err != nil {
				return fmt.Errorf("第 %d 条命令失败: %w", i+1, err)
			}
			report("log", fmt.Sprintf("命令 %d/%d · 成功", i+1, len(st.Commands)), "", 0, 0)
		}
		return nil
	}
	for {
		err = r.Run(ctx, st, func(msg string) { report("log", msg, "", 0, 0) })
		if err == nil || st.Type != "health" {
			return err
		}
		var unknown *UnknownError
		if errors.As(err, &unknown) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(st.Interval) * time.Second):
		}
	}
}
