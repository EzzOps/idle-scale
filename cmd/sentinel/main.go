package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	port := getEnv("IDLE_SERVICE_PORT", "8080")
	ignoreRaw := getEnv("IDLE_IGNORE_PATHS", "/healthz,/readyz,/livez,/metrics")
	ignoreSet := make(map[string]bool)
	for _, p := range strings.Split(ignoreRaw, ",") {
		ignoreSet[strings.TrimSpace(p)] = true
	}

	addr := fmt.Sprintf(":%s", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sentinel: listen %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sentinel: accept: %v\n", err)
			continue
		}
		go handleConn(conn, ignoreSet)
	}
}

func handleConn(conn net.Conn, ignoreSet map[string]bool) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	reader := bufio.NewReaderSize(conn, 512)
	buf, err := reader.Peek(512)
	if err != nil {
		wakeUp()
		return
	}
	if isHTTP(buf) {
		path := extractPath(string(buf))
		if path != "" && ignoreSet[path] {
			return
		}
	}
	wakeUp()
}

func isHTTP(buf []byte) bool {
	if len(buf) < 4 {
		return false
	}
	methods := [][]byte{
		[]byte("GET "), []byte("POST "), []byte("PUT "),
		[]byte("PATC"), []byte("DELE"), []byte("HEAD"),
		[]byte("OPTI"), []byte("CONN"), []byte("TRAC"),
	}
	for _, m := range methods {
		if len(buf) >= len(m) && string(buf[:len(m)]) == string(m) {
			return true
		}
	}
	return false
}

func extractPath(req string) string {
	parts := strings.SplitN(req, " ", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func wakeUp() {
	fmt.Fprintln(os.Stderr, "sentinel: traffic detected, exiting")
	os.Exit(42)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
