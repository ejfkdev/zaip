package proxy

import (
	"bufio"
	"io"
	"log"
	"net"
	"time"

	"github.com/ejfkdev/zaip/internal/tunnel"
)

type Listener struct {
	addr     string
	client   *tunnel.Client
	listener net.Listener
}

func NewListener(addr string, client *tunnel.Client) *Listener {
	return &Listener{addr: addr, client: client}
}

func (l *Listener) Start() error {
	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return err
	}
	l.listener = ln
	log.Printf("proxy listener on %s (HTTP/HTTPS/SOCKS4/SOCKS5)", l.addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go l.handleConn(conn)
	}
}

func (l *Listener) Close() {
	if l.listener != nil {
		_ = l.listener.Close()
	}
}

func (l *Listener) handleConn(conn net.Conn) {
	br := bufio.NewReader(conn)
	b, err := br.Peek(1)
	if err != nil {
		_ = conn.Close()
		return
	}

	bc := &bufferedConn{Conn: conn, reader: br}

	switch {
	case b[0] == 0x05:
		l.handleSOCKS5(bc)
	case b[0] == 0x04:
		l.handleSOCKS4(bc)
	default:
		l.handleHTTP(bc)
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

// relay does bidirectional copy between a local conn and a tunnel stream.
// Waits for both directions to finish with a timeout.
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
