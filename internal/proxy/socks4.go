package proxy

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"strconv"
)

const (
	socks4Version    = 0x04
	socks4CmdConnect = 0x01

	socks4Granted  = 0x5A
	socks4Rejected = 0x5B
)

func (l *Listener) handleSOCKS4(conn *bufferedConn) {
	defer conn.Close()

	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != socks4Version || header[1] != socks4CmdConnect {
		sendSOCKS4Reply(conn, socks4Rejected)
		return
	}

	port := binary.BigEndian.Uint16(header[2:4])
	ip := net.IP(header[4:8])

	// Read null-terminated user ID
	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		if b[0] == 0 {
			break
		}
	}

	var addr string
	// SOCKS4a: IP 0.0.0.x means domain name follows
	if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] != 0 {
		var domain []byte
		for {
			b := make([]byte, 1)
			if _, err := io.ReadFull(conn, b); err != nil {
				return
			}
			if b[0] == 0 {
				break
			}
			domain = append(domain, b[0])
		}
		addr = string(domain)
	} else {
		addr = ip.String()
	}

	log.Printf("socks4 connect to %s:%d", addr, port)

	// Open tunnel to target through server
	stream, err := l.client.OpenTunnel(addr, port)
	if err != nil {
		log.Printf("socks4 connect %s:%d failed: %v", addr, port, err)
		sendSOCKS4Reply(conn, socks4Rejected)
		return
	}

	// Send granted reply to client
	sendSOCKS4Reply(conn, socks4Granted)

	// Bidirectional relay
	relay(conn, stream)
}

func sendSOCKS4Reply(conn net.Conn, code uint8) {
	_, _ = conn.Write([]byte{0x00, code, 0, 0, 0, 0, 0, 0})
}

// parseHostPort is in http.go but socks4 also needs it — use the shared one.
// This is just a reminder that parseHostPort is shared; no duplicate needed.
var _ = strconv.Itoa // socks4.go uses strconv only if we had local parseHostPort
