package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"

	"github.com/flowgate/flowgate/internal/node/forwarder"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var code int
	switch os.Args[1] {
	case "ws-roundtrip":
		code = runWSRoundtrip(os.Args[2:])
	case "tls-echo-server":
		code = runTLSEchoServer(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		usage()
		code = 2
	}

	os.Exit(code)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  tunnel_helper ws-roundtrip --url ws://127.0.0.1:12345/ws --message hello")
	fmt.Fprintln(os.Stderr, "  tunnel_helper tls-echo-server --addr 127.0.0.1:12345")
}

func runWSRoundtrip(args []string) int {
	fs := flag.NewFlagSet("ws-roundtrip", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	url := fs.String("url", "", "WebSocket URL")
	message := fs.String("message", "", "message payload")
	timeout := fs.Duration("timeout", 5*time.Second, "dial/read timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *url == "" {
		fmt.Fprintln(os.Stderr, "--url is required")
		return 2
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: *timeout,
	}
	conn, _, err := dialer.Dial(*url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws dial failed: %v\n", err)
		return 1
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(*timeout))
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte(*message)); err != nil {
		fmt.Fprintf(os.Stderr, "ws write failed: %v\n", err)
		return 1
	}

	_ = conn.SetReadDeadline(time.Now().Add(*timeout))
	_, reply, err := conn.ReadMessage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws read failed: %v\n", err)
		return 1
	}

	if _, err := os.Stdout.Write(reply); err != nil {
		fmt.Fprintf(os.Stderr, "stdout write failed: %v\n", err)
		return 1
	}

	return 0
}

func runTLSEchoServer(args []string) int {
	fs := flag.NewFlagSet("tls-echo-server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	addr := fs.String("addr", "", "listen address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *addr == "" {
		fmt.Fprintln(os.Stderr, "--addr is required")
		return 2
	}

	cert, err := forwarder.GenerateSelfSignedCert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate cert failed: %v\n", err)
		return 1
	}

	ln, err := tls.Listen("tcp", *addr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "tls listen failed: %v\n", err)
		return 1
	}
	defer ln.Close()

	log.Printf("tls echo server listening on %s", *addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				continue
			}
			fmt.Fprintf(os.Stderr, "accept failed: %v\n", err)
			return 1
		}

		go func(c net.Conn) {
			defer c.Close()
			_, _ = io.Copy(c, c)
		}(conn)
	}
}
