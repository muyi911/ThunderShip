package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countedSession struct {
	closed atomic.Int32
	run    func(context.Context, Step) error
}

func (s *countedSession) Close()                      { s.closed.Add(1) }
func (s *countedSession) CheckFiles([]Artifact) error { return nil }
func (s *countedSession) Publish(context.Context, []Artifact, Step, func(string, int64, int64, string)) error {
	return nil
}
func (s *countedSession) Run(ctx context.Context, st Step, _ func(string)) error {
	if s.run != nil {
		return s.run(ctx, st)
	}
	return nil
}

func TestRunSharesSessionsAcrossNodesAndNestedWorkflows(t *testing.T) {
	var connected atomic.Int32
	var completed atomic.Int32
	var mu sync.Mutex
	var sessions []*countedSession
	e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) {
		connected.Add(1)
		s := &countedSession{run: func(context.Context, Step) error { completed.Add(1); return nil }}
		mu.Lock()
		sessions = append(sessions, s)
		mu.Unlock()
		return s, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c := EmptyConfig()
	c.Servers = []Server{{ID: "s", Host: "host", User: "user", Port: 22}}
	w := workflow("child", "restart")
	w.Steps = append(w.Steps, Step{ID: "second", Type: "command", Command: "verify", Timeout: 3})
	parent := NormalizeWorkflow(workflow("parent", "first"))
	parent.Steps = append(parent.Steps, Step{ID: "call", Name: "child", Type: "workflow", WorkflowID: "child"})
	parent.Flow = nil
	c.Workflows = []Workflow{parent, w}
	root := Node{Mode: "parallel", Children: []Node{{WorkflowID: "parent"}, {WorkflowID: "child"}}}
	for i := 1; i <= 2; i++ {
		if err := e.node(context.Background(), c, root, "run"); err != nil {
			t.Fatal(err)
		}
		if got := connected.Load(); got != int32(i) {
			t.Fatalf("run %d: opened %d connections, want %d", i, got, i)
		}
		if sessions[i-1].closed.Load() != 1 {
			t.Fatal("run did not close its session exactly once")
		}
	}
	if completed.Load() != 10 {
		t.Fatalf("lost steps: %d", completed.Load())
	}
}

func TestSessionWaitTimeoutDoesNotCancelSharedDial(t *testing.T) {
	var connected atomic.Int32
	ready := make(chan struct{})
	started := make(chan struct{})
	session := &countedSession{}
	e, err := NewEngine(t.TempDir(), func(ctx context.Context, _ Server) (Session, error) {
		if connected.Add(1) == 1 {
			close(started)
		}
		select {
		case <-ready:
			return session, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, closeSessions := e.withSessions(context.Background())
	defer closeSessions()
	pool := ctx.Value(sessionScopeKey{}).(*sessionScope)
	s := Server{ID: "server"}
	short, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := pool.get(short, s); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(ready)
	got, err := pool.get(ctx, s)
	if err != nil || got != session || connected.Load() != 1 {
		t.Fatal("a waiting node canceled the shared dial", got, err, connected.Load())
	}
	s.SudoUpload = true
	if _, err := pool.get(ctx, s); err != nil || connected.Load() != 1 {
		t.Fatal("sudo opened another SSH transport", err)
	}
	s.User = "other-user"
	if _, err := pool.get(ctx, s); err != nil || connected.Load() != 2 {
		t.Fatal("different login reused a session", err)
	}
}

func TestFailedConnectionIsNotRepeatedForParallelNodes(t *testing.T) {
	var connected atomic.Int32
	e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) {
		connected.Add(1)
		return nil, errors.New("connection refused")
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, closeSessions := e.withSessions(context.Background())
	defer closeSessions()
	pool := ctx.Value(sessionScopeKey{}).(*sessionScope)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := pool.get(ctx, Server{ID: "server"}); err == nil {
				t.Error("connection failure lost")
			}
		}()
	}
	wg.Wait()
	if connected.Load() != 1 {
		t.Fatalf("parallel dial storm: %d connections", connected.Load())
	}
}

func TestRunCancellationClosesSharedSession(t *testing.T) {
	started := make(chan struct{})
	session := &countedSession{run: func(ctx context.Context, _ Step) error {
		close(started)
		<-ctx.Done()
		return &UnknownError{ctx.Err()}
	}}
	e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) { return session, nil })
	if err != nil {
		t.Fatal(err)
	}
	c := EmptyConfig()
	c.Servers = []Server{{ID: "s"}}
	c.Workflows = []Workflow{workflow("w", "wait")}
	done := make(chan Event, 1)
	e.OnEvent = func(ev Event) {
		if ev.Workflow == "" {
			done <- ev
		}
	}
	id, err := e.Start(context.Background(), c, Node{WorkflowID: "w"}, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	e.Cancel(id)
	select {
	case ev := <-done:
		if ev.Status != "unknown" || session.closed.Load() != 1 {
			t.Fatal("cancellation lost unknown status or leaked connection", ev, session.closed.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not finish")
	}
}
