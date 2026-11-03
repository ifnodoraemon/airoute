package router

import (
	"context"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

type lanePolicy struct {
	limit, queue int
	wait         time.Duration
}

type laneWaiter struct {
	ready   chan struct{}
	granted bool
}

type upstreamLane struct {
	busy    int
	waiters []*laneWaiter
}

// callLanes bounds upstream calls for each endpoint/credential on this replica.
// A slot belongs to the response body, including the full lifetime of a stream.
type callLanes struct {
	mu       sync.Mutex
	policies map[string]lanePolicy
	lanes    map[string]*upstreamLane
}

func newCallLanes(channels []model.ChannelConfig) *callLanes {
	l := &callLanes{lanes: make(map[string]*upstreamLane)}
	l.configure(channels)
	return l
}

func (l *callLanes) configure(channels []model.ChannelConfig) {
	policies := make(map[string]lanePolicy)
	for i := range channels {
		ch := &channels[i]
		if ch.MaxConcurrency <= 0 || (ch.Status != "" && ch.Status != "active") {
			continue
		}
		wait := time.Duration(ch.QueueWaitSeconds) * time.Second
		if wait <= 0 {
			wait = 5 * time.Second
		}
		key := upstreamIdentity(ch)
		p := lanePolicy{limit: ch.MaxConcurrency, queue: max(ch.QueueLimit, 0), wait: wait}
		if old, ok := policies[key]; ok {
			p.limit, p.queue, p.wait = min(p.limit, old.limit), min(p.queue, old.queue), min(p.wait, old.wait)
		}
		policies[key] = p
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.policies = policies
	for key, lane := range l.lanes {
		l.grantLocked(key, lane)
		if lane.busy == 0 && len(lane.waiters) == 0 {
			delete(l.lanes, key)
		}
	}
}

func (l *callLanes) acquire(ctx context.Context, ch *model.ChannelConfig) (func(), error) {
	key := upstreamIdentity(ch)
	l.mu.Lock()
	p, limited := l.policies[key]
	if !limited {
		l.mu.Unlock()
		return func() {}, ctx.Err()
	}
	lane := l.lanes[key]
	if lane == nil {
		lane = &upstreamLane{}
		l.lanes[key] = lane
	}
	if ctx.Err() != nil {
		l.mu.Unlock()
		return nil, ctx.Err()
	}
	if lane.busy < p.limit && len(lane.waiters) == 0 {
		lane.busy++
		l.mu.Unlock()
		return l.releaser(key, lane), nil
	}
	if len(lane.waiters) >= p.queue {
		l.mu.Unlock()
		return nil, &admissionError{reason: "upstream concurrency queue is full; no upstream was called", until: time.Now().Add(time.Second)}
	}
	w := &laneWaiter{ready: make(chan struct{})}
	lane.waiters = append(lane.waiters, w)
	l.mu.Unlock()
	return l.wait(ctx, key, lane, w, p.wait)
}

func (l *callLanes) wait(ctx context.Context, key string, lane *upstreamLane, w *laneWaiter, wait time.Duration) (func(), error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	var err error
	select {
	case <-w.ready:
		err = ctx.Err()
	case <-ctx.Done():
		err = ctx.Err()
	case <-timer.C:
		err = &admissionError{reason: "upstream concurrency queue wait expired; no upstream was called", until: time.Now().Add(time.Second)}
	}
	if err == nil {
		return l.releaser(key, lane), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if w.granted {
		lane.busy--
	} else {
		for i, queued := range lane.waiters {
			if queued == w {
				lane.waiters = append(lane.waiters[:i], lane.waiters[i+1:]...)
				break
			}
		}
	}
	l.grantLocked(key, lane)
	return nil, err
}

func (l *callLanes) releaser(key string, lane *upstreamLane) func() {
	return sync.OnceFunc(func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		lane.busy--
		l.grantLocked(key, lane)
		if lane.busy == 0 && len(lane.waiters) == 0 {
			delete(l.lanes, key)
		}
	})
}

func (l *callLanes) grantLocked(key string, lane *upstreamLane) {
	p, limited := l.policies[key]
	for len(lane.waiters) > 0 && (!limited || lane.busy < p.limit) {
		w := lane.waiters[0]
		lane.waiters = lane.waiters[1:]
		w.granted = true
		lane.busy++
		close(w.ready)
	}
}
