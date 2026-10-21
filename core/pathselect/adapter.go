package pathselect

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

type round struct {
	life   context.Context
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
	failures   int
	verified   bool
	healthyFor time.Duration
	lastUse    time.Time
	sessions   map[*session]struct{}
	udpRoute   *Adapter
}

func New(name string, cfg Config, candidates []C.Proxy) *Adapter {
	a := &Adapter{Base: outbound.NewBase(outbound.BaseOption{Name: name, Addr: "auto-tcp", Type: C.Socks5, UDP: cfg.UDP}),
		candidates: append([]C.Proxy(nil), candidates...), endpoint: cfg.ProbeURL, check: probe,
		healthyFor: 5 * time.Minute, sessions: make(map[*session]struct{})}
	if cfg.Version == 2 {
		a.verified, a.check, a.healthyFor = true, probeTransfer, time.Minute
	}
	if cfg.UDP {
		cfg.UDP = false
		var udp []C.Proxy
		for _, candidate := range candidates {
			if candidate.SupportUDP() {
				udp = append(udp, candidate)
			}
		}
		a.udpRoute = New(name, cfg, udp)
		a.udpRoute.check = probeUDP
	}
	return a
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
	a.failures = 0
	if active && !a.closed {
		a.life, a.stop = context.WithCancel(context.Background())
		if a.verified {
			a.lastUse = time.Now()
			go a.monitor(a.life)
		}
	}
}

func (a *Adapter) SetActive(active bool) {
	if a.udpRoute != nil {
		a.udpRoute.SetActive(active)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if active && a.life != nil {
		return
	}
	a.resetLocked(active)
}

func (a *Adapter) Reset() {
	if a.udpRoute != nil {
		a.udpRoute.Reset()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resetLocked(a.life != nil)
}

func (a *Adapter) Close() error {
	if a.udpRoute != nil {
		_ = a.udpRoute.Close()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	a.resetLocked(false)
	return nil
}

func (a *Adapter) choose(ctx context.Context) (C.Proxy, context.Context, error) {
	return a.selectRoute(ctx, true)
}

func (a *Adapter) selectRoute(ctx context.Context, touch bool) (C.Proxy, context.Context, error) {
	a.mu.Lock()
	if a.life == nil || a.closed {
		a.mu.Unlock()
		return nil, nil, errors.New("AutoTCP is stopped")
	}
	life := a.life
	if touch {
		a.lastUse = time.Now()
	}
	if a.selected != nil && time.Now().Before(a.validUntil) &&
		(touch || !a.verified || time.Now().Before(a.validUntil.Add(-refreshLead))) {
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
		g = &round{life: life, ctx: probeCtx, cancel: cancel, done: make(chan struct{})}
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
	winner, preferredFailed, pending := a.selectCandidate(g, preferred)
	a.mu.Lock()
	var retired []*session
	var watch bool
	if a.current == g && g.ctx.Err() == nil && a.life != nil {
		a.current = nil
		a.selected = winner
		if winner == nil {
			if a.verified {
				a.selected = preferred
				a.validUntil = time.Time{}
			}
			a.backoffLocked()
			log.Warnln("[AutoTCP] no candidate completed the transfer check")
		} else {
			a.validUntil = time.Now().Add(a.healthyFor)
			a.failures, a.retryAfter = 0, time.Time{}
			g.winner = winner
			log.Infoln("[AutoTCP] verified path: %s", winner.Name())
			if a.verified && preferred != nil && preferred != winner {
				for s := range a.sessions {
					if s.route == preferred && s.life == a.life {
						if preferredFailed {
							retired = append(retired, s)
						}
						watch = pending != nil
					}
				}
			}
		}
	} else if a.current == g {
		a.current = nil
		a.backoffLocked()
	}
	a.mu.Unlock()
	close(g.done)
	if pending != nil {
		if watch {
			go a.watchPreferred(g.life, preferred, pending)
		} else {
			pending.cancel()
		}
	}
	for _, s := range retired {
		_ = s.Close()
	}
}

func (a *Adapter) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	for attempt := 0; attempt < 2; attempt++ {
		selected, life, err := a.choose(ctx)
		if err != nil {
			return nil, err
		}
		dialCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(life, cancel)
		conn, err := selected.DialContext(dialCtx, metadata)
		canceled := dialCtx.Err() != nil
		stop()
		cancel()
		if err != nil {
			if !canceled {
				a.invalidate(life, selected)
			}
			if a.verified && attempt == 0 && !canceled {
				continue
			}
			return nil, errors.New("AutoTCP destination connection failed")
		}
		if life.Err() != nil || ctx.Err() != nil {
			_ = conn.Close()
			return nil, errors.New("AutoTCP stopped or network changed")
		}
		conn.AppendToChains(a)
		if a.verified {
			return &trackedConn{Conn: conn, session: a.track(conn, selected, life)}, nil
		}
		return conn, nil
	}
	return nil, errors.New("AutoTCP destination connection failed")
}

func (a *Adapter) MarshalJSON() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := ""
	if a.selected != nil {
		now = a.selected.Name()
	}
	return json.Marshal(map[string]any{"type": a.Type().String(), "now": now, "udp": a.SupportUDP()})
}

type session struct {
	owner *Adapter
	conn  io.Closer
	route C.Proxy
	life  context.Context
	once  sync.Once
	stop  func() bool
}

func (s *session) Close() error {
	var err error
	s.once.Do(func() {
		s.owner.mu.Lock()
		delete(s.owner.sessions, s)
		if s.stop != nil {
			s.stop()
		}
		s.owner.mu.Unlock()
		err = s.conn.Close()
	})
	return err
}

type trackedConn struct {
	C.Conn
	session *session
}

func (c *trackedConn) Close() error            { return c.session.Close() }
func (c *trackedConn) Upstream() any           { return c.Conn }
func (c *trackedConn) ReaderReplaceable() bool { return true }
func (c *trackedConn) WriterReplaceable() bool { return true }

type trackedPacketConn struct {
	C.PacketConn
	session *session
}

func (c *trackedPacketConn) Close() error            { return c.session.Close() }
func (c *trackedPacketConn) Upstream() any           { return c.PacketConn }
func (c *trackedPacketConn) ReaderReplaceable() bool { return true }
func (c *trackedPacketConn) WriterReplaceable() bool { return true }

func (a *Adapter) track(conn io.Closer, route C.Proxy, life context.Context) *session {
	s := &session{owner: a, conn: conn, route: route, life: life}
	a.mu.Lock()
	a.sessions[s] = struct{}{}
	a.lastUse = time.Now()
	s.stop = context.AfterFunc(life, func() { _ = s.Close() })
	a.mu.Unlock()
	return s
}

func (a *Adapter) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	if a.udpRoute == nil {
		return nil, C.ErrNotSupport
	}
	u := a.udpRoute
	for attempt := 0; attempt < 2; attempt++ {
		selected, life, err := u.choose(ctx)
		if err != nil {
			return nil, err
		}
		dialCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(life, cancel)
		conn, err := selected.ListenPacketContext(dialCtx, metadata)
		canceled := dialCtx.Err() != nil
		stop()
		cancel()
		if err != nil {
			if !canceled {
				u.invalidate(life, selected)
			}
			if u.verified && attempt == 0 && !canceled {
				continue
			}
			return nil, errors.New("verified UDP destination connection failed")
		}
		if life.Err() != nil || ctx.Err() != nil {
			_ = conn.Close()
			return nil, errors.New("automatic route stopped or network changed")
		}
		conn.AppendToChains(a)
		return &trackedPacketConn{PacketConn: conn, session: u.track(conn, selected, life)}, nil
	}
	return nil, errors.New("verified UDP destination connection failed")
}
