package tunnel

import (
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ejfkdev/zaip/internal/protocol"
)

type Server struct {
	addr     string
	upgrader websocket.Upgrader
	server   *http.Server
}

func NewServer(addr string) *Server {
	return &Server{
		addr: addr,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  64 * 1024,
			WriteBufferSize: 64 * 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/proxy", s.handleWebSocket)

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	log.Printf("tunnel server listening on %s", s.addr)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Close() {
	if s.server != nil {
		_ = s.server.Close()
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}

	// Read deadline for CONNECT phase
	_ = wsConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	ws := newWSStream(wsConn)

	// Read CONNECT command byte
	cmd := make([]byte, 1)
	if _, err := io.ReadFull(ws, cmd); err != nil {
		_ = wsConn.Close()
		return
	}
	if cmd[0] != protocol.CmdConnect {
		_, _ = ws.Write([]byte{protocol.CmdConnected, protocol.StatusRefused})
		_ = wsConn.Close()
		return
	}

	// Read ConnectRequest: 3-byte head + addr + port
	head := make([]byte, 3)
	if _, err := io.ReadFull(ws, head); err != nil {
		_ = wsConn.Close()
		return
	}
	addrLen := int(head[2])
	remaining := make([]byte, addrLen+2)
	if _, err := io.ReadFull(ws, remaining); err != nil {
		_ = wsConn.Close()
		return
	}

	reqData := append(head, remaining...)
	req, err := protocol.ReadConnectRequest(reqData)
	if err != nil {
		_, _ = ws.Write([]byte{protocol.CmdConnected, protocol.StatusRefused})
		_ = wsConn.Close()
		return
	}

	// Dial target
	targetConn, err := net.DialTimeout(req.Network(), req.Address(), 3*time.Second)
	if err != nil {
		_, _ = ws.Write([]byte{protocol.CmdConnected, protocol.StatusRefused})
		_ = wsConn.Close()
		return
	}
	defer targetConn.Close()

	// TCP_NODELAY for lower latency
	if tc, ok := targetConn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	// Send CONNECTED OK
	if _, err := ws.Write([]byte{protocol.CmdConnected, protocol.StatusOK}); err != nil {
		_ = wsConn.Close()
		return
	}

	// Clear read deadline for relay phase
	_ = wsConn.SetReadDeadline(time.Time{})

	go func() {
		_, _ = io.Copy(ws, targetConn)
		_ = ws.Close()
	}()
	_, _ = io.Copy(targetConn, ws)
	_ = targetConn.Close()
}
