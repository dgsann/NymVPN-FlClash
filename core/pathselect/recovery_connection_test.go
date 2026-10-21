package pathselect

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
)

func TestRecoveryRefreshRunsBeforeExpiryWithoutHoldingNewConnections(t *testing.T) {
	route := fake("live")
	a := New("verified", Config{Version: 2}, []C.Proxy{route})
	started := make(chan struct{})
	a.check = func(ctx context.Context, _ C.Proxy, _ string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	a.SetActive(true)
	defer a.Close()
	a.mu.Lock()
	a.selected = route
	a.validUntil = time.Now().Add(10 * time.Second)
	life := a.life
	a.mu.Unlock()
	go a.recheck(life)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh waited for cache expiry")
	}
	conn, err := a.DialContext(context.Background(), &C.Metadata{})
	if err != nil {
		t.Fatal("live connection waited for background refresh:", err)
	}
	conn.Close()
}

type recoveryPacketAdapter struct {
	*fakeAdapter
	listen func(context.Context, *C.Metadata) (C.PacketConn, error)
}

func (a *recoveryPacketAdapter) ListenPacketContext(ctx context.Context, m *C.Metadata) (C.PacketConn, error) {
	return a.listen(ctx, m)
}

func TestRecoveryRetriesUDPAssociationBeforeReturningError(t *testing.T) {
	var failed atomic.Bool
	old := &recoveryPacketAdapter{fakeAdapter: &fakeAdapter{Base: outbound.NewBase(outbound.BaseOption{Name: "old", Type: C.Hysteria2, UDP: true})}}
	backup := &recoveryPacketAdapter{fakeAdapter: &fakeAdapter{Base: outbound.NewBase(outbound.BaseOption{Name: "backup", Type: C.Hysteria2, UDP: true})}}
	old.listen = func(context.Context, *C.Metadata) (C.PacketConn, error) {
		failed.Store(true)
		return nil, errors.New("association lost")
	}
	backup.listen = func(context.Context, *C.Metadata) (C.PacketConn, error) {
		return outbound.NewPacketConn(&dnsPacket{mode: "valid", closed: make(chan struct{})}, backup), nil
	}
	first, second := adapter.NewProxy(old), adapter.NewProxy(backup)
	a := New("verified", Config{Version: 2, UDP: true}, []C.Proxy{first, second})
	a.check = func(context.Context, C.Proxy, string) error { return nil }
	a.udpRoute.check = func(_ context.Context, p C.Proxy, _ string) error {
		if p == first && !failed.Load() || p == second && failed.Load() {
			return nil
		}
		return errors.New("blocked")
	}
	a.SetActive(true)
	defer a.Close()
	conn, err := a.ListenPacketContext(context.Background(), &C.Metadata{NetWork: C.UDP})
	if err != nil {
		t.Fatal("available UDP backup was not tried:", err)
	}
	conn.Close()
	if choose(t, a.udpRoute) != second {
		t.Fatal("UDP backup was not retained")
	}
}
