package pathselect

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

type round struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	winner C.Proxy
}

type Adapter struct {
	*outbound.Base
	mu         sync.Mutex
	candidates []C.Proxy
	endpoint   string
	check      func(context.Context, C.Proxy, string) error
	closed     bool
	life       context.Context
	stop       context.CancelFunc
	current    *round
	selected   C.Proxy
	validUntil time.Time
	retryAfter time.Time
}

func New(name string, cfg Config, candidates []C.Proxy) *Adapter {
	return &Adapter{Base: outbound.NewBase(outbound.BaseOption{Name: name, Addr: "auto-tcp", Type: C.Socks5}),
		candidates: append([]C.Proxy(nil), candidates...), endpoint: cfg.ProbeURL, check: probe}
}

func (a *Adapter) resetLocked(active bool) {
	if a.stop != nil {
		a.stop()
	}
	if a.current != nil {
		a.current.cancel()
	}
	a.life, a.stop, a.current, a.selected = nil, nil, nil, nil
	a.validUntil, a.retryAfter = time.Time{}, time.Time{}
	if active && !a.closed {
		a.life, a.stop = context.WithCancel(context.Background())
	}
}

func (a *Adapter) SetActive(active bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if active && a.life != nil {
		return
	}
	a.resetLocked(active)
}

func (a *Adapter) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resetLocked(a.life != nil)
}

func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	a.resetLocked(false)
	return nil
}

func (a *Adapter) choose(ctx context.Context) (C.Proxy, context.Context, error) {
	a.mu.Lock()
	if a.life == nil || a.closed {
		a.mu.Unlock()
		return nil, nil, errors.New("AutoTCP is stopped")
	}
	life := a.life
	if a.selected != nil && time.Now().Before(a.validUntil) {
		selected := a.selected
		a.mu.Unlock()
		return selected, life, nil
	}
	if time.Now().Before(a.retryAfter) {
		a.mu.Unlock()
		return nil, nil, errors.New("AutoTCP is waiting to retry")
	}
	g := a.current
	if g == nil {
		probeCtx, cancel := context.WithTimeout(life, 50*time.Second)
		g = &round{ctx: probeCtx, cancel: cancel, done: make(chan struct{})}
		a.current = g
		go a.run(g, a.selected)
	}
	a.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-life.Done():
		return nil, nil, errors.New("AutoTCP stopped or network changed")
	case <-g.done:
	}
	if life.Err() != nil {
		return nil, nil, errors.New("AutoTCP stopped or network changed")
	}
	if g.winner == nil {
		return nil, nil, errors.New("AutoTCP has no verified path")
	}
	return g.winner, life, nil
}

func (a *Adapter) run(g *round, preferred C.Proxy) {
	defer g.cancel()
	var winner C.Proxy
	if preferred != nil && a.check(g.ctx, preferred, a.endpoint) == nil {
		winner = preferred
	}
	if winner == nil && g.ctx.Err() == nil {
		results := make(chan C.Proxy, len(a.candidates))
		count := 0
		for _, candidate := range a.candidates {
			if candidate == preferred {
				continue
			}
			count++
			go func(p C.Proxy) {
				if a.check(g.ctx, p, a.endpoint) == nil {
					results <- p
				} else {
					results <- nil
				}
			}(candidate)
		}
		for range count {
			select {
			case p := <-results:
				if p != nil {
					winner = p
				}
			case <-g.ctx.Done():
			}
			if winner != nil || g.ctx.Err() != nil {
				break
			}
		}
	}
	a.mu.Lock()
	if a.current == g && g.ctx.Err() == nil && a.life != nil {
		a.current = nil
		a.selected = winner
		if winner == nil {
			a.retryAfter = time.Now().Add(15 * time.Second)
			log.Warnln("[AutoTCP] no candidate completed the transfer check")
		} else {
			a.validUntil = time.Now().Add(5 * time.Minute)
			g.winner = winner
			log.Infoln("[AutoTCP] verified path: %s", winner.Name())
		}
	} else if a.current == g {
		a.current = nil
		a.retryAfter = time.Now().Add(15 * time.Second)
	}
	a.mu.Unlock()
	close(g.done)
}

func (a *Adapter) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	selected, life, err := a.choose(ctx)
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(life, cancel)
	defer stop()
	defer cancel()
	conn, err := selected.DialContext(dialCtx, metadata)
	if err != nil {
		if dialCtx.Err() == nil {
			a.mu.Lock()
			if a.life == life && a.selected == selected {
				a.validUntil = time.Time{}
			}
			a.mu.Unlock()
		}
		return nil, errors.New("AutoTCP destination connection failed")
	}
	if life.Err() != nil {
		conn.Close()
		return nil, errors.New("AutoTCP stopped or network changed")
	}
	conn.AppendToChains(a)
	return conn, nil
}

func (a *Adapter) MarshalJSON() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := ""
	if a.selected != nil {
		now = a.selected.Name()
	}
	return json.Marshal(map[string]any{"type": a.Type().String(), "now": now})
}
