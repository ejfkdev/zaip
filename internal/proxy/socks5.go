package proxy

import (
	"encoding/binary"
	"io"
	"log"
	"net"
)

const (
	socks5Version = 0x05

	socks5AuthNone     = 0x00
	socks5AuthNoAccept = 0xFF

	socks5CmdConnect = 0x01

	socks5AtypIPv4   = 0x01
	socks5AtypDomain = 0x03
	socks5AtypIPv6   = 0x04

	socks5RepSuccess        = 0x00
	socks5RepServerFailure  = 0x01
	socks5RepNotAllowed     = 0x02
	socks5RepNetUnreachable = 0x03
	socks5RepHostUnreachable = 0x04
	socks5RepConnRefused    = 0x05
	socks5RepCmdNotSupport  = 0x07
	socks5RepAtypNotSupport = 0x08
)

func (l *Listener) handleSOCKS5(conn *bufferedConn) {
	defer conn.Close()

	// Auth negotiation
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	if buf[0] != socks5Version {
		return
	}
	methods := make([]byte, buf[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	hasNoAuth := false
	for _, m := range methods {
		if m == socks5AuthNone {
			hasNoAuth = true
			break
		}
	}
	if !hasNoAuth {
		_, _ = conn.Write([]byte{socks5Version, socks5AuthNoAccept})
		return
	}
	_, _ = conn.Write([]byte{socks5Version, socks5AuthNone})

	// Read connect request
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != socks5Version || header[1] != socks5CmdConnect {
		sendSOCKS5Reply(conn, socks5RepCmdNotSupport, nil, 0)
		return
	}

	var addr string
	var port uint16

	switch header[3] {
	case socks5AtypIPv4:
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, ipBuf); err != nil {
			return
		}
		addr = net.IP(ipBuf).String()

	case socks5AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
		addr = string(domain)

	case socks5AtypIPv6:
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(conn, ipBuf); err != nil {
			return
		}
		addr = net.IP(ipBuf).String()

	default:
		sendSOCKS5Reply(conn, socks5RepAtypNotSupport, nil, 0)
		return
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port = binary.BigEndian.Uint16(portBuf)

	// Open tunnel to target through server
	stream, err := l.client.OpenTunnel(addr, port)
	if err != nil {
		log.Printf("socks5 connect %s:%d failed: %v", addr, port, err)
		sendSOCKS5Reply(conn, socks5RepHostUnreachable, nil, 0)
		return
	}

	// Send success reply to client
	sendSOCKS5Reply(conn, socks5RepSuccess, nil, 0)

	// Bidirectional relay
	relay(conn, stream)
}

func sendSOCKS5Reply(conn net.Conn, rep uint8, bindAddr net.IP, bindPort uint16) {
	resp := []byte{socks5Version, rep, 0x00, socks5AtypIPv4}
	if bindAddr == nil {
		resp = append(resp, 0, 0, 0, 0)
	} else {
		resp = append(resp, bindAddr.To4()...)
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, bindPort)
	resp = append(resp, portBytes...)
	_, _ = conn.Write(resp)
}
