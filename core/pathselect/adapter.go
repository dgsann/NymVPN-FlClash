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
	if active && !a.closed {
		a.life, a.stop = context.WithCancel(context.Background())
		if a.verified {
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
		jobs := make(chan C.Proxy, len(a.candidates))
		count := 0
		for _, candidate := range a.candidates {
			if candidate == preferred {
				continue
			}
			count++
			jobs <- candidate
		}
		close(jobs)
		for range min(4, count) {
			go func() {
				for p := range jobs {
					if g.ctx.Err() != nil {
						return
					}
					if a.check(g.ctx, p, a.endpoint) == nil {
						results <- p
					} else {
						results <- nil
					}
				}
			}()
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
	var retired []*session
	if a.current == g && g.ctx.Err() == nil && a.life != nil {
		a.current = nil
		a.selected = winner
		if winner == nil {
			if a.verified {
				a.selected = preferred
				a.validUntil = time.Time{}
			}
			a.retryAfter = time.Now().Add(15 * time.Second)
			log.Warnln("[AutoTCP] no candidate completed the transfer check")
		} else {
			a.validUntil = time.Now().Add(a.healthyFor)
			g.winner = winner
			log.Infoln("[AutoTCP] verified path: %s", winner.Name())
			if a.verified && preferred != nil && preferred != winner {
				for s := range a.sessions {
					if s.route == preferred && s.life == a.life {
						retired = append(retired, s)
					}
				}
			}
		}
	} else if a.current == g {
		a.current = nil
		a.retryAfter = time.Now().Add(15 * time.Second)
	}
	a.mu.Unlock()
	close(g.done)
	for _, s := range retired {
		_ = s.Close()
	}
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
	if a.verified {
		s := a.track(conn, selected, life)
		return &trackedConn{Conn: conn, session: s}, nil
	}
	return conn, nil
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

func (a *Adapter) monitor(life context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-life.Done():
			return
		case <-ticker.C:
			a.recheck(life)
		}
	}
}

func (a *Adapter) recheck(life context.Context) {
	a.mu.Lock()
	active := a.life == life && (len(a.sessions) > 0 || time.Since(a.lastUse) < 2*time.Minute)
	due := time.Now().After(a.validUntil)
	a.mu.Unlock()
	if active && due {
		_, _, _ = a.choose(life)
	}
}

func (a *Adapter) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	if a.udpRoute == nil {
		return nil, C.ErrNotSupport
	}
	u := a.udpRoute
	selected, life, err := u.choose(ctx)
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(life, cancel)
	defer stop()
	defer cancel()
	conn, err := selected.ListenPacketContext(dialCtx, metadata)
	if err != nil {
		u.mu.Lock()
		if u.life == life && u.selected == selected {
			u.validUntil = time.Time{}
		}
		u.mu.Unlock()
		return nil, errors.New("verified UDP destination connection failed")
	}
	if life.Err() != nil {
		_ = conn.Close()
		return nil, errors.New("automatic route stopped or network changed")
	}
	conn.AppendToChains(a)
	return &trackedPacketConn{PacketConn: conn, session: u.track(conn, selected, life)}, nil
}
