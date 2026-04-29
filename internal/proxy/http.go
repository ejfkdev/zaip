package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

func (l *Listener) handleHTTP(conn *bufferedConn) {
	defer conn.Close()

	req, err := http.ReadRequest(conn.reader)
	if err != nil {
		return
	}

	if req.Method == http.MethodConnect {
		l.handleHTTPS(conn, req)
		return
	}

	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	proxyHost, proxyPort := parseHostPort(host, 80)

	stream, err := l.client.OpenTunnel(proxyHost, proxyPort)
	if err != nil {
		log.Printf("http proxy %s failed: %v", host, err)
		return
	}

	// Rewrite absolute URL to relative path for origin server
	req.URL.Scheme = ""
	req.URL.Host = ""
	req.RequestURI = ""

	// Write request to target (streams body)
	if err := req.Write(stream); err != nil {
		_ = stream.Close()
		return
	}

	// Bidirectional relay for response and subsequent data
	relay(conn, stream)
}

func (l *Listener) handleHTTPS(conn *bufferedConn, req *http.Request) {
	host := req.URL.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}
	proxyHost, proxyPort := parseHostPort(host, 443)

	stream, err := l.client.OpenTunnel(proxyHost, proxyPort)
	if err != nil {
		log.Printf("https proxy %s failed: %v", host, err)
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}

	_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	relay(conn, stream)
}

func parseHostPort(host string, defaultPort int) (string, uint16) {
	hostname, portStr, err := net.SplitHostPort(host)
	if err != nil {
		return host, uint16(defaultPort)
	}
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	return hostname, port
}
