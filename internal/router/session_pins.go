package router

import (
	"container/list"
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
)

const maxSessionPins = 8192
const affinityRedisTimeout = 100 * time.Millisecond

type sessionPinKey struct{ model, session string }
type sessionPin struct {
	key     sessionPinKey
	channel string
	expires time.Time
}

// sessionPins remembers successful channels even in deployments without Redis.
// Both the lifetime and the number of entries are bounded.
type sessionPins struct {
	mu      sync.Mutex
	entries map[sessionPinKey]*list.Element
	order   list.List
	ttl     time.Duration
	now     func() time.Time
}

func newSessionPins() *sessionPins {
	ttl := 30 * time.Minute
	if n, err := strconv.Atoi(os.Getenv("GATEWAY_SESSION_AFFINITY_TTL_SEC")); err == nil && n > 0 && n <= 86400 {
		ttl = time.Duration(n) * time.Second
	}
	return &sessionPins{entries: make(map[sessionPinKey]*list.Element), ttl: ttl, now: time.Now}
}

func (p *sessionPins) get(model, session string) string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entries[sessionPinKey{model, session}]
	if e == nil {
		return ""
	}
	pin := e.Value.(sessionPin)
	if !p.now().Before(pin.expires) {
		delete(p.entries, pin.key)
		p.order.Remove(e)
		return ""
	}
	p.order.MoveToFront(e)
	return pin.channel
}

func (p *sessionPins) put(model, session, channel string) {
	if p == nil || session == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key := sessionPinKey{model, session}
	pin := sessionPin{key: key, channel: channel, expires: p.now().Add(p.ttl)}
	if e := p.entries[key]; e != nil {
		e.Value = pin
		p.order.MoveToFront(e)
		return
	}
	if p.order.Len() >= maxSessionPins {
		e := p.order.Back()
		delete(p.entries, e.Value.(sessionPin).key)
		p.order.Remove(e)
	}
	p.entries[key] = p.order.PushFront(pin)
}

func (p *sessionPins) forget(model, session, channel string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key := sessionPinKey{model, session}
	if e := p.entries[key]; e != nil && e.Value.(sessionPin).channel == channel {
		delete(p.entries, key)
		p.order.Remove(e)
	}
}

func sessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if s, _ := ctx.Value(ContextKeySessionID).(string); s != "" {
		return s
	}
	s, _ := ctx.Value("session_id").(string)
	return s
}

func (d *Dispatcher) rememberSessionChannel(ctx context.Context, modelName, channel string) {
	session := sessionIDFromContext(ctx)
	if session == "" {
		return
	}
	d.sessionPins.put(modelName, session, channel)
	if client := distributed.GetClient(); client != nil && client.IsActive() {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), affinityRedisTimeout)
		defer cancel()
		_ = client.SetSessionChannel(writeCtx, modelName, session, channel, d.sessionPins.ttl)
	}
}

func (d *Dispatcher) forgetSessionChannel(ctx context.Context, modelName, channel string) {
	session := sessionIDFromContext(ctx)
	if session == "" {
		return
	}
	d.sessionPins.forget(modelName, session, channel)
	if client := distributed.GetClient(); client != nil && client.IsActive() {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), affinityRedisTimeout)
		defer cancel()
		_ = client.DeleteSessionChannelIfMatches(writeCtx, modelName, session, channel)
	}
}
