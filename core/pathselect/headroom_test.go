package pathselect

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/buf"
	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/vless/vision"
)

type paddingConn struct {
	N.ExtendedConn
	written []byte
}

func (c *paddingConn) FrontHeadroom() int { return vision.PaddingHeaderLen }
func (c *paddingConn) RearHeadroom() int  { return 1400 }
func (c *paddingConn) WriteBuffer(buffer *buf.Buffer) error {
	defer buffer.Release()
	vision.ApplyPadding(buffer, 0, nil, true)
	c.written = append([]byte(nil), buffer.Bytes()...)
	return nil
}

func TestTrackedConnectionPreservesVisionRelayBuffers(t *testing.T) {
	route := fake("vision")
	a := New("verified", Config{Version: 2}, []C.Proxy{route})
	a.check = func(context.Context, C.Proxy, string) error { return nil }
	a.SetActive(true)
	defer a.Close()
	left, right := net.Pipe()
	defer right.Close()
	raw := &paddingConn{ExtendedConn: N.NewExtendedConn(left)}
	route.Adapter().(*fakeAdapter).dial = func(context.Context, *C.Metadata) (C.Conn, error) {
		return outbound.NewConn(raw, route.Adapter().(*fakeAdapter)), nil
	}
	conn, err := a.DialContext(context.Background(), &C.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	payload := bytes.Repeat([]byte{0x42}, 128)
	options := N.NewReadWaitOptions(nil, conn)
	buffer := options.NewBuffer()
	_, _ = buffer.Write(payload)
	if err := conn.WriteBuffer(buffer); err != nil {
		t.Fatal(err)
	}
	if len(raw.written) < len(payload)+5 || !bytes.Equal(raw.written[5:5+len(payload)], payload) {
		t.Fatal("relay payload or Vision padding corrupted")
	}
	if N.UnwrapWriter(conn) != raw || N.UnwrapReader(conn) != raw {
		t.Fatal("transport capabilities hidden from relay")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	remaining := len(a.sessions)
	a.mu.Unlock()
	if remaining != 0 {
		t.Fatal("relay close lost session ownership")
	}
}

type paddedPacket struct{ C.PacketConn }

func (*paddedPacket) FrontHeadroom() int { return 21 }
func (*paddedPacket) RearHeadroom() int  { return 1400 }

func TestTrackedPacketPreservesTransportHeadroom(t *testing.T) {
	packet := &trackedPacketConn{PacketConn: &paddedPacket{}}
	options := N.NewReadWaitOptions(nil, packet)
	if options.FrontHeadroom != 21 || options.RearHeadroom != 1400 {
		t.Fatal("UDP transport headroom hidden by session tracking")
	}
}
