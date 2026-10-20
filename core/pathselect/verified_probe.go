package pathselect

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/miekg/dns"
)

func probeTransfer(ctx context.Context, candidate C.Proxy, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	payload := make([]byte, ProbeSize)
	if _, err := rand.Read(payload); err != nil {
		return errors.New("transfer probe initialization failed")
	}
	transport := &http.Transport{
		DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 8192,
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			host, portText, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("invalid probe destination")
			}
			port, err := strconv.ParseUint(portText, 10, 16)
			if err != nil {
				return nil, errors.New("invalid probe destination")
			}
			return candidate.DialContext(ctx, &C.Metadata{NetWork: C.TCP, Host: host, DstPort: uint16(port)})
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("invalid transfer probe request")
	}
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("transfer probe connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "" {
		return errors.New("transfer probe response rejected")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ProbeSize+1))
	if err != nil || len(body) != ProbeSize || sha256.Sum256(body) != sha256.Sum256(payload) {
		return errors.New("transfer probe payload incomplete or invalid")
	}
	return nil
}

func probeUDP(ctx context.Context, candidate C.Proxy, _ string) error {
	if !candidate.SupportUDP() {
		return C.ErrNotSupport
	}
	for _, host := range []string{"1.1.1.1", "8.8.8.8"} {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if probeDNS(ctx, candidate, host) == nil {
			return nil
		}
	}
	return errors.New("UDP transfer probe failed")
}

func probeDNS(ctx context.Context, candidate C.Proxy, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	m := &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr(host), DstPort: 53}
	conn, err := candidate.ListenPacketContext(ctx, m)
	if err != nil {
		return errors.New("UDP probe connection failed")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	query := new(dns.Msg)
	query.SetQuestion("example.com.", dns.TypeA)
	data, err := query.Pack()
	if err != nil {
		return err
	}
	destination := &net.UDPAddr{IP: net.ParseIP(host), Port: 53}
	if _, err := conn.WriteTo(data, destination); err != nil {
		return err
	}
	buffer := make([]byte, 4096)
	n, from, err := conn.ReadFrom(buffer)
	if err != nil {
		return err
	}
	if from.String() != destination.String() {
		return errors.New("unexpected UDP probe source")
	}
	response := new(dns.Msg)
	if response.Unpack(buffer[:n]) != nil || response.Id != query.Id || !response.Response ||
		response.Rcode != dns.RcodeSuccess || response.Truncated || len(response.Answer) == 0 ||
		len(response.Question) != 1 || response.Question[0] != query.Question[0] {
		return errors.New("invalid UDP probe response")
	}
	return nil
}
