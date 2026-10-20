package pathselect

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/miekg/dns"
)

type dnsPacket struct {
	mode   string
	reply  []byte
	from   net.Addr
	closed chan struct{}
	once   sync.Once
}

func (p *dnsPacket) WriteTo(data []byte, addr net.Addr) (int, error) {
	query := new(dns.Msg)
	if err := query.Unpack(data); err != nil {
		return 0, err
	}
	response := new(dns.Msg)
	response.SetReply(query)
	response.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.IPv4(192, 0, 2, 1)}}
	if p.mode == "wrong-id" {
		response.Id++
	}
	if p.mode == "empty" {
		response.Answer = nil
	}
	if p.mode == "wrong-question" {
		response.Question[0].Name = "other.test."
	}
	p.reply, _ = response.Pack()
	p.from = addr
	if p.mode == "wrong-source" {
		p.from = &net.UDPAddr{IP: net.IPv4(192, 0, 2, 2), Port: 53}
	}
	return len(data), nil
}

func (p *dnsPacket) ReadFrom(b []byte) (int, net.Addr, error) {
	if p.mode == "stall" {
		<-p.closed
		return 0, nil, net.ErrClosed
	}
	return copy(b, p.reply), p.from, nil
}
func (p *dnsPacket) Close() error                     { p.once.Do(func() { close(p.closed) }); return nil }
func (p *dnsPacket) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (p *dnsPacket) SetDeadline(time.Time) error      { return nil }
func (p *dnsPacket) SetReadDeadline(time.Time) error  { return nil }
func (p *dnsPacket) SetWriteDeadline(time.Time) error { return nil }

type dnsAdapter struct {
	*outbound.Base
	packet *dnsPacket
}

func (a *dnsAdapter) ListenPacketContext(context.Context, *C.Metadata) (C.PacketConn, error) {
	return outbound.NewPacketConn(a.packet, a), nil
}

func TestUDPProbeRequiresAuthenticatedCandidateAndMatchingResponse(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-id", "empty", "wrong-question", "wrong-source", "stall"} {
		t.Run(mode, func(t *testing.T) {
			p := &dnsPacket{mode: mode, closed: make(chan struct{})}
			a := &dnsAdapter{Base: outbound.NewBase(outbound.BaseOption{Name: "udp", Type: C.Hysteria2, UDP: true}), packet: p}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			err := probeDNS(ctx, adapter.NewProxy(a), "1.1.1.1")
			if (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected result: %v", err)
			}
			select {
			case <-p.closed:
			default:
				t.Fatal("probe socket leaked")
			}
		})
	}
	if !errors.Is(probeUDP(context.Background(), fake("tcp"), ""), C.ErrNotSupport) {
		t.Fatal("TCP-only candidate accepted")
	}
}
