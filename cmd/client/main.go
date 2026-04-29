package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ejfkdev/zaip/internal/proxy"
	"github.com/ejfkdev/zaip/internal/tunnel"
)

var version = "dev"

// resolveServerURL converts user input to a WebSocket URL.
// Accepts: https://host, http://host, wss://host, ws://host, bare hostname
// Default path: /proxy
func resolveServerURL(input string) string {
	hasScheme := strings.HasPrefix(input, "http://") ||
		strings.HasPrefix(input, "https://") ||
		strings.HasPrefix(input, "ws://") ||
		strings.HasPrefix(input, "wss://")

	if !hasScheme {
		input = "https://" + input
	}

	u, err := url.Parse(input)
	if err != nil {
		log.Fatalf("invalid URL: %v", err)
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	if u.Path == "" || u.Path == "/" {
		u.Path = "/proxy"
	}

	return u.String()
}

// findAvailablePort tries to listen on bindAddr:startPort,
// increments port if occupied, returns the working address and port.
func findAvailablePort(bindAddr string, startPort int) (string, int) {
	for port := startPort; port < startPort+100; port++ {
		addr := fmt.Sprintf("%s:%d", bindAddr, port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return addr, port
		}
	}
	log.Fatalf("no available port from %d on %s", startPort, bindAddr)
	return "", 0
}

func main() {
	bind := flag.String("bind", "0.0.0.0", "Bind address")
	port := flag.Int("port", 7890, "Local proxy port (auto-increments if occupied)")
	conns := flag.Int("conns", 5, "Number of WSS connections to server")
	showVersion := flag.Bool("v", false, "Print version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "zaip-client - WebSocket Tunnel Proxy Client  https://github.com/ejfkdev/zaip\n\n")
		fmt.Fprintf(os.Stderr, "Usage: zaip-client [options] <server-url>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nServer URL formats:\n")
		fmt.Fprintf(os.Stderr, "  https://xxx-d.space-z.ai     (https -> wss, path: /proxy)\n")
		fmt.Fprintf(os.Stderr, "  wss://example.com/custom     (wss with custom path)\n")
		fmt.Fprintf(os.Stderr, "  http://192.168.1.100:3000    (http -> ws)\n")
		fmt.Fprintf(os.Stderr, "  example.com                  (bare host, defaults to wss)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  zaip-client https://xxx-d.space-z.ai\n")
		fmt.Fprintf(os.Stderr, "  zaip-client -port 8080 wss://example.com/tunnel\n")
		fmt.Fprintf(os.Stderr, "  zaip-client -bind 127.0.0.1 -conns 10 example.com\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	serverURL := resolveServerURL(flag.Arg(0))

	pool := tunnel.NewSessionPool(serverURL, *conns)
	if err := pool.Start(); err != nil {
		log.Fatalf("session pool start failed: %v", err)
	}

	client := tunnel.NewClient(pool)

	listenAddr, actualPort := findAvailablePort(*bind, *port)
	ln := proxy.NewListener(listenAddr, client)

	go func() {
		if err := ln.Start(); err != nil {
			log.Fatalf("proxy listener failed: %v", err)
		}
	}()

	log.Printf("zaip-client %s started on :%d -> %s (conns: %d)", version, actualPort, serverURL, *conns)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down...")
	ln.Close()
	pool.Close()
}
