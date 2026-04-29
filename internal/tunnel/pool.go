package tunnel

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// SessionPool maintains a warm pool of WSS connections.
// Each request consumes one connection; the pool replenishes automatically.
type SessionPool struct {
	serverURL string
	dialer    websocket.Dialer
	poolSize  int
	conns     chan *websocket.Conn
	closed    chan struct{}
	dialSem   chan struct{}
}

func NewSessionPool(serverURL string, poolSize int) *SessionPool {
	return &SessionPool{
		serverURL: serverURL,
		dialer:    websocket.Dialer{HandshakeTimeout: 3 * time.Second},
		poolSize:  poolSize,
		conns:     make(chan *websocket.Conn, poolSize),
		closed:    make(chan struct{}),
		dialSem:   make(chan struct{}, 5),
	}
}

func (p *SessionPool) Start() error {
	for i := 0; i < p.poolSize; i++ {
		conn, err := p.dial()
		if err != nil {
			return err
		}
		p.conns <- conn
	}
	go p.refill()
	go p.rotate()
	log.Printf("connection pool started, pool size: %d", p.poolSize)
	return nil
}

// Take returns a warm connection, or dials a new one if pool is empty.
func (p *SessionPool) Take() (*websocket.Conn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dial()
	}
}

func (p *SessionPool) Close() {
	close(p.closed)
	for {
		select {
		case conn := <-p.conns:
			_ = conn.Close()
		default:
			return
		}
	}
}

func (p *SessionPool) dial() (*websocket.Conn, error) {
	select {
	case p.dialSem <- struct{}{}:
		defer func() { <-p.dialSem }()
	case <-p.closed:
		return nil, fmt.Errorf("pool closed")
	}

	conn, _, err := p.dialer.Dial(p.serverURL, nil)
	if err == nil {
		return conn, nil
	}
	if isTLSError(err) {
		wsURL := strings.Replace(p.serverURL, "wss://", "ws://", 1)
		log.Printf("TLS failed, trying ws:// %s", wsURL)
		conn, _, err2 := p.dialer.Dial(wsURL, nil)
		if err2 == nil {
			return conn, nil
		}
	}
	return nil, err
}

func isTLSError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "TLS") ||
		strings.Contains(msg, "tls:") ||
		strings.Contains(msg, "certificate")
}

// refill continuously keeps the pool at poolSize.
// Blocks when pool is full, unblocks when a slot opens.
func (p *SessionPool) refill() {
	for {
		conn, err := p.dial()
		if err != nil {
			log.Printf("pool dial failed: %v", err)
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-p.closed:
				return
			}
		}
		select {
		case p.conns <- conn:
		case <-p.closed:
			_ = conn.Close()
			return
		}
	}
}

// rotate periodically evicts one connection to prevent stale connections.
func (p *SessionPool) rotate() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			select {
			case conn := <-p.conns:
				_ = conn.Close()
			default:
			}
		case <-p.closed:
			return
		}
	}
}
