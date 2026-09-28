package pathselect

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

const ProbeSize = 64 * 1024

var probeDigest = func() [32]byte {
	data := make([]byte, ProbeSize)
	for i := range data {
		data[i] = byte(i)
	}
	return sha256.Sum256(data)
}()

func probe(ctx context.Context, candidate C.Proxy, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	transport := &http.Transport{
		DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 8192,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("invalid probe request")
	}
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("AutoTCP probe connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "" {
		return errors.New("AutoTCP probe response rejected")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, ProbeSize+1))
	if err != nil || len(data) != ProbeSize || sha256.Sum256(data) != probeDigest {
		return errors.New("AutoTCP probe payload incomplete or invalid")
	}
	return nil
}
