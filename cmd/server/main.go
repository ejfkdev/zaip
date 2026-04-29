package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ejfkdev/zaip/internal/tunnel"
)

var version = "dev"

func main() {
	addr := flag.String("addr", ":3000", "Listen address")
	showVersion := flag.Bool("v", false, "Print version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "zaip-server - WebSocket Tunnel Proxy Server  https://github.com/ejfkdev/zaip\n\n")
		fmt.Fprintf(os.Stderr, "Usage: zaip-server [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	srv := tunnel.NewServer(*addr)
	if err := srv.Start(); err != nil {
		log.Fatalf("server start failed: %v", err)
	}

	log.Printf("zaip-server %s started on %s", version, *addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down...")
	srv.Close()
}
