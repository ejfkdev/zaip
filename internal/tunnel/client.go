package tunnel

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/ejfkdev/zaip/internal/protocol"
)

// Client provides tunnel connections via multiple pools with load balancing.
type Client struct {
	pools []*SessionPool
	rand  *rand.Rand
}

func NewClient(pools []*SessionPool) *Client {
	return &Client{
		pools: pools,
		rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ProxyConn opens a tunnel and relays bidirectionally.
func (c *Client) ProxyConn(localConn net.Conn, targetAddr string, targetPort uint16) error {
	stream, err := c.OpenTunnel(targetAddr, targetPort)
	if err != nil {
		return err
	}
	relay(localConn, stream)
	log.Printf("proxy done: %s:%d", targetAddr, targetPort)
	return nil
}

// OpenTunnel picks a pool randomly and tries to open a tunnel.
// On failure, tries remaining pools (failover).
func (c *Client) OpenTunnel(targetAddr string, targetPort uint16) (io.ReadWriteCloser, error) {
	start := c.rand.Intn(len(c.pools))
	var lastErr error
	for i := range c.pools {
		idx := (start + i) % len(c.pools)
		stream, err := c.openTunnelOnPool(c.pools[idx], targetAddr, targetPort)
		if err == nil {
			return stream, nil
		}
		lastErr = err
		if isStaleError(err) {
			c.pools[idx].Drain()
		}
		if isRefusedError(err) {
			// Server refused the target, other pools will likely refuse too
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) openTunnelOnPool(pool *SessionPool, targetAddr string, targetPort uint16) (io.ReadWriteCloser, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		stream, err := c.openTunnelOnce(pool, targetAddr, targetPort)
		if err == nil {
			return stream, nil
		}
		lastErr = err
		if isStaleError(err) {
			pool.Drain()
			continue
		}
		if isRefusedError(err) {
			return nil, err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, lastErr
}

func (c *Client) openTunnelOnce(pool *SessionPool, targetAddr string, targetPort uint16) (io.ReadWriteCloser, error) {
	conn, err := pool.Take()
	if err != nil {
		return nil, fmt.Errorf("take connection: %w", err)
	}

	ws := newWSStream(conn)

	req := &protocol.ConnectRequest{
		Addr:     targetAddr,
		Port:     targetPort,
		AddrType: protocol.AddrType(targetAddr),
	}

	msg := append([]byte{protocol.CmdConnect}, req.Marshal()...)
	if _, err := ws.Write(msg); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send connect: %w", err)
	}

	buf := make([]byte, 2)
	if _, err := io.ReadFull(ws, buf); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read connected: %w", err)
	}
	if buf[0] != protocol.CmdConnected || buf[1] != protocol.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("connect refused: cmd=0x%02x status=0x%02x", buf[0], buf[1])
	}

	return ws, nil
}

func isStaleError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "1006") ||
		strings.Contains(msg, "unexpected EOF") ||
		strings.Contains(msg, "broken pipe")
}

func isRefusedError(err error) bool {
	return strings.Contains(err.Error(), "connect refused")
}

func relay(conn net.Conn, stream io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(stream, conn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, stream)
		done <- struct{}{}
	}()
	<-done
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	_ = stream.Close()
	_ = conn.Close()
}
