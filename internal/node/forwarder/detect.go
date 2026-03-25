package forwarder

import (
	"net"
	"strings"
	"time"
)

// PeekConn wraps a net.Conn with peeked bytes that are replayed on first Read calls.
type PeekConn struct {
	net.Conn
	buf []byte // peeked bytes to replay
	pos int    // current position in buf
}

// Read replays peeked bytes first, then reads from the underlying connection.
func (pc *PeekConn) Read(p []byte) (int, error) {
	if pc.pos < len(pc.buf) {
		n := copy(p, pc.buf[pc.pos:])
		pc.pos += n
		return n, nil
	}
	return pc.Conn.Read(p)
}

// PeekBytes reads up to n bytes from conn with a short deadline, returning a
// PeekConn that will replay those bytes on subsequent reads.
func PeekBytes(conn net.Conn, n int) (*PeekConn, []byte, error) {
	buf := make([]byte, n)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	nr, err := conn.Read(buf)
	conn.SetReadDeadline(time.Time{}) // clear deadline

	if nr == 0 && err != nil {
		return nil, nil, err
	}

	peeked := buf[:nr]
	pc := &PeekConn{
		Conn: conn,
		buf:  peeked,
		pos:  0,
	}
	return pc, peeked, nil
}

// DetectProtocol identifies the protocol from the first bytes of a connection.
// Returns: "socks5", "socks4", "http", "tls", or "unknown".
func DetectProtocol(firstBytes []byte) string {
	if len(firstBytes) == 0 {
		return "unknown"
	}

	switch firstBytes[0] {
	case 0x05:
		return "socks5"
	case 0x04:
		return "socks4"
	case 0x16:
		return "tls"
	}

	// HTTP method detection
	if len(firstBytes) >= 3 {
		prefix := string(firstBytes[:min(8, len(firstBytes))])
		httpMethods := []string{"GET ", "POST", "HEAD", "PUT ", "DELETE", "CONNECT", "OPTIONS", "PATCH"}
		for _, m := range httpMethods {
			if strings.HasPrefix(prefix, m) {
				return "http"
			}
		}
	}

	return "unknown"
}

// IsBlocked checks if the detected protocol is in the blocked list.
// blockedList is comma-separated, e.g. "socks,http".
// "socks" matches both "socks4" and "socks5".
func IsBlocked(detected, blockedList string) bool {
	if blockedList == "" || detected == "unknown" {
		return false
	}

	for _, b := range strings.Split(blockedList, ",") {
		b = strings.TrimSpace(strings.ToLower(b))
		if b == "" {
			continue
		}
		if b == detected {
			return true
		}
		// "socks" blocks both socks4 and socks5
		if b == "socks" && (detected == "socks4" || detected == "socks5") {
			return true
		}
	}
	return false
}
