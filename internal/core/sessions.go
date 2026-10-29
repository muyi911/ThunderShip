package core

import (
	"context"
	"sync"
)

type sessionScopeKey struct{}

type sessionEntry struct {
	ready chan struct{}
	value Session
	err   error
}

// A scope belongs to one run, including every nested workflow.
type sessionScope struct {
	ctx     context.Context
	cancel  context.CancelFunc
	connect Connector
	mu      sync.Mutex
	closed  bool
	entries map[Server]*sessionEntry
}

func (e *Engine) withSessions(ctx context.Context) (context.Context, func()) {
	if ctx.Value(sessionScopeKey{}) != nil {
		return ctx, func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &sessionScope{ctx: ctx, cancel: cancel, connect: e.connect, entries: map[Server]*sessionEntry{}}
	return context.WithValue(ctx, sessionScopeKey{}, s), s.close
}

func (s *sessionScope) get(ctx context.Context, server Server) (Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// SFTP privilege belongs to the upload channel, not the SSH login.
	server.SudoUpload = false
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, context.Canceled
	}
	entry := s.entries[server]
	if entry != nil {
		select {
		case <-entry.ready:
			if remote, ok := entry.value.(interface{ isClosed() bool }); ok && remote.isClosed() {
				entry = nil
			}
		default:
		}
	}
	if entry == nil {
		entry = &sessionEntry{ready: make(chan struct{})}
		s.entries[server] = entry
		go func() {
			entry.value, entry.err = s.connect(s.ctx, server)
			close(entry.ready)
		}()
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		if s.ctx.Err() != nil {
			<-entry.ready
			if entry.err != nil {
				return nil, entry.err
			}
		}
		return nil, ctx.Err()
	case <-entry.ready:
		if entry.err != nil {
			return nil, entry.err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return entry.value, nil
	}
}

func (s *sessionScope) transfer(ctx context.Context, server Server) (Session, func(), error) {
	r, err := s.get(ctx, server)
	if err != nil {
		return nil, nil, err
	}
	if opener, ok := r.(interface {
		OpenTransfer(context.Context, Server) (Session, error)
	}); ok {
		transfer, err := opener.OpenTransfer(ctx, server)
		if err != nil {
			return nil, nil, err
		}
		return transfer, transfer.Close, nil
	}
	return r, func() {}, nil
}

func (s *sessionScope) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cancel()
	entries := s.entries
	s.mu.Unlock()
	for _, entry := range entries {
		<-entry.ready
		if entry.value != nil {
			entry.value.Close()
		}
	}
}
