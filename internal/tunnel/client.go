package tunnel

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/ejfkdev/zaip/internal/protocol"
)

// Client provides tunnel connections via a pool of WSS connections.
type Client struct {
	pool *SessionPool
}

func NewClient(pool *SessionPool) *Client {
	return &Client{pool: pool}
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

// OpenTunnel takes a WSS connection, sends CONNECT, reads CONNECTED.
// Retries once on failure.
func (c *Client) OpenTunnel(targetAddr string, targetPort uint16) (io.ReadWriteCloser, error) {
	stream, err := c.openTunnelOnce(targetAddr, targetPort)
	if err != nil {
		log.Printf("tunnel attempt failed, retrying: %v", err)
		stream, err = c.openTunnelOnce(targetAddr, targetPort)
		if err != nil {
			return nil, err
		}
	}
	return stream, nil
}

func (c *Client) openTunnelOnce(targetAddr string, targetPort uint16) (io.ReadWriteCloser, error) {
	conn, err := c.pool.Take()
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
