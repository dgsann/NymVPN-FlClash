package telemost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/client"
	"golang.org/x/net/proxy"
)

type runner func(context.Context, client.Config, func(string)) error

type generation struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	done     chan struct{}
	address  string
	username string
	password string
}

type Adapter struct {
	*outbound.Base
	mu         sync.Mutex
	config     Config
	run        runner
	active     bool
	closed     bool
	current    *generation
	retryAfter time.Time
}

func New(name string, cfg Config) *Adapter {
	return &Adapter{
		Base:   outbound.NewBase(outbound.BaseOption{Name: name, Addr: "telemost", Type: C.Socks5}),
		config: cfg,
		run: func(ctx context.Context, cfg client.Config, ready func(string)) error {
			return client.New(cfg).RunWithAddress(ctx, ready)
		},
	}
}

func (a *Adapter) SetActive(active bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = active && !a.closed
	if !a.active && a.current != nil {
		a.current.cancel()
	}
}

func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	a.active = false
	if a.current != nil {
		a.current.cancel()
	}
	return nil
}

func (a *Adapter) session() (*generation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.active || a.closed {
		return nil, errors.New("Telemost transport is stopped")
	}
	if a.current != nil {
		select {
		case <-a.current.done:
			a.current = nil
		default:
			if a.current.ctx.Err() != nil {
				return nil, errors.New("Telemost transport is stopping")
			}
			return a.current, nil
		}
	}
	if time.Now().Before(a.retryAfter) {
		return nil, errors.New("Telemost transport is waiting to reconnect")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, errors.New("Telemost local authentication initialization failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &generation{ctx: ctx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}), username: "nymvpn", password: hex.EncodeToString(secret)}
	a.current = g
	go a.serve(g)
	return g, nil
}

func (a *Adapter) serve(g *generation) {
	defer func() {
		if recover() != nil {
			log.Errorln("[Telemost] transport stopped unexpectedly")
		}
		g.cancel()
		a.mu.Lock()
		a.retryAfter = time.Now().Add(3 * time.Second)
		a.mu.Unlock()
		close(g.done)
	}()
	cfg := client.Config{
		Provider: "telemost", Transport: "vp8channel", RoomURL: a.config.Room,
		KeyHex: a.config.Key, DNSServer: a.config.DNS, LocalAddr: "127.0.0.1:0",
		SOCKSUser: g.username, SOCKSPass: g.password,
		TransportOptions: client.VP8Options{FPS: 30, BatchSize: 64},
		Liveness:         client.LivenessConfig{Interval: 10 * time.Second, Timeout: 15 * time.Second, Failures: 4},
	}
	var readyOnce sync.Once
	err := a.run(g.ctx, cfg, func(address string) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host != "127.0.0.1" || port == "0" || g.ctx.Err() != nil {
			g.cancel()
			return
		}
		readyOnce.Do(func() {
			g.address = address
			close(g.ready)
			log.Infoln("[Telemost] tunnel is ready")
		})
	})
	if err != nil && g.ctx.Err() == nil {
		log.Warnln("[Telemost] connection failed; retry available after a short backoff")
	}
}

func (a *Adapter) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	g, err := a.session()
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.ctx.Done():
		return nil, errors.New("Telemost transport disconnected")
	case <-g.ready:
	}
	dialer, err := proxy.SOCKS5("tcp", g.address, &proxy.Auth{User: g.username, Password: g.password}, &net.Dialer{})
	if err != nil {
		return nil, errors.New("Telemost local proxy initialization failed")
	}
	connectionCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	defer cancel()
	conn, err := dialer.(proxy.ContextDialer).DialContext(connectionCtx, "tcp", metadata.RemoteAddress())
	if err != nil {
		return nil, errors.New("Telemost connection to destination failed")
	}
	return outbound.NewConn(conn, a), nil
}
