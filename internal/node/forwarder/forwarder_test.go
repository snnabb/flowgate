package forwarder

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestDetectProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "socks5", data: []byte{0x05, 0x01, 0x00}, want: "socks5"},
		{name: "socks4", data: []byte{0x04, 0x01, 0x00}, want: "socks4"},
		{name: "http", data: []byte("GET / HTTP/1.1\r\n"), want: "http"},
		{name: "tls", data: []byte{0x16, 0x03, 0x01}, want: "tls"},
		{name: "unknown", data: []byte{0x01, 0x02, 0x03}, want: "unknown"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectProtocol(tt.data); got != tt.want {
				t.Fatalf("DetectProtocol(%v) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

func TestIsBlockedTreatsSocksAsAggregate(t *testing.T) {
	t.Parallel()

	if !IsBlocked("socks5", "socks,http") {
		t.Fatal("expected socks5 to be blocked by socks aggregate")
	}
	if !IsBlocked("http", "socks,http") {
		t.Fatal("expected http to be blocked")
	}
	if IsBlocked("tls", "socks,http") {
		t.Fatal("expected tls to remain allowed")
	}
}

func TestPeekBytesTreatsTimeoutAsUnknownProtocol(t *testing.T) {
	t.Parallel()

	conn := &timeoutConn{}

	peekConn, peeked, err := PeekBytes(conn, 8)
	if err != nil {
		t.Fatalf("PeekBytes returned error on timeout: %v", err)
	}
	if peekConn == nil {
		t.Fatal("PeekBytes returned nil PeekConn on timeout")
	}
	if len(peeked) != 0 {
		t.Fatalf("expected no peeked bytes on timeout, got %d", len(peeked))
	}
	if got := DetectProtocol(peeked); got != "unknown" {
		t.Fatalf("DetectProtocol(timeout) = %q, want unknown", got)
	}
}

func TestConnPoolGetReturnsHealthyConnectionWithoutProbeRead(t *testing.T) {
	t.Parallel()

	pooled := &readTrackingConn{}
	pool := NewConnPool(1, time.Minute, func() (net.Conn, error) {
		return &readTrackingConn{}, nil
	})
	pool.pool <- &pooledConn{Conn: pooled, idleSince: time.Now()}

	got, err := pool.Get()
	if err != nil {
		t.Fatalf("ConnPool.Get returned error: %v", err)
	}
	if got != pooled {
		t.Fatal("ConnPool.Get did not return the pooled connection")
	}
	if pooled.reads != 0 {
		t.Fatalf("expected pooled connection to avoid probe reads, got %d", pooled.reads)
	}
}

func TestConnPoolGetFallsBackToFreshDialAfterStaleConn(t *testing.T) {
	t.Parallel()

	stale := &readTrackingConn{}
	fresh := &readTrackingConn{}
	dials := 0
	pool := NewConnPool(1, time.Millisecond, func() (net.Conn, error) {
		dials++
		return fresh, nil
	})
	pool.pool <- &pooledConn{Conn: stale, idleSince: time.Now().Add(-time.Second)}

	got, err := pool.Get()
	if err != nil {
		t.Fatalf("ConnPool.Get returned error: %v", err)
	}
	if got != fresh {
		t.Fatal("ConnPool.Get did not fall back to a fresh connection")
	}
	if !stale.closed {
		t.Fatal("expected stale pooled connection to be closed")
	}
	if dials != 1 {
		t.Fatalf("expected one fresh dial, got %d", dials)
	}
}

func TestWriteProxyV1(t *testing.T) {
	t.Parallel()

	dst := &bufferConn{}
	clientAddr := &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 45678}
	localAddr := &net.TCPAddr{IP: net.ParseIP("198.51.100.20"), Port: 19000}

	if err := WriteProxyV1(dst, clientAddr, localAddr); err != nil {
		t.Fatalf("WriteProxyV1 returned error: %v", err)
	}

	want := "PROXY TCP4 203.0.113.10 198.51.100.20 45678 19000\r\n"
	if got := dst.String(); got != want {
		t.Fatalf("WriteProxyV1 wrote %q, want %q", got, want)
	}
}

func TestWriteProxyV2(t *testing.T) {
	t.Parallel()

	dst := &bufferConn{}
	clientAddr := &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 45678}
	localAddr := &net.TCPAddr{IP: net.ParseIP("198.51.100.20"), Port: 19000}

	if err := WriteProxyV2(dst, clientAddr, localAddr); err != nil {
		t.Fatalf("WriteProxyV2 returned error: %v", err)
	}

	data := dst.Bytes()
	if len(data) != 28 {
		t.Fatalf("expected 28-byte IPv4 PROXY v2 header, got %d", len(data))
	}
	if !bytes.Equal(data[:12], proxyV2Sig[:]) {
		t.Fatal("PROXY v2 signature mismatch")
	}
	if data[12] != 0x21 {
		t.Fatalf("unexpected version/command byte: 0x%x", data[12])
	}
	if data[13] != 0x11 {
		t.Fatalf("unexpected family/protocol byte: 0x%x", data[13])
	}
}

func TestNewTLSListenerAndDial(t *testing.T) {
	t.Parallel()

	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer baseListener.Close()

	tlsListener, err := NewTLSListener(baseListener)
	if err != nil {
		t.Fatalf("NewTLSListener failed: %v", err)
	}
	defer tlsListener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := tlsListener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			serverDone <- err
			return
		}
		if _, err := conn.Write([]byte("pong")); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	clientConn, err := TLSDial(baseListener.Addr().String(), 5*time.Second, "")
	if err != nil {
		t.Fatalf("TLSDial failed: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("ping")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("client read failed: %v", err)
	}
	if string(reply) != "pong" {
		t.Fatalf("unexpected TLS reply %q", reply)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("TLS server failed: %v", err)
	}
}

type timeoutConn struct{}

func (c *timeoutConn) Read([]byte) (int, error)         { return 0, timeoutError{} }
func (c *timeoutConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *timeoutConn) Close() error                     { return nil }
func (c *timeoutConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *timeoutConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *timeoutConn) SetDeadline(time.Time) error      { return nil }
func (c *timeoutConn) SetReadDeadline(time.Time) error  { return nil }
func (c *timeoutConn) SetWriteDeadline(time.Time) error { return nil }

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type readTrackingConn struct {
	reads  int
	closed bool
}

func (c *readTrackingConn) Read([]byte) (int, error) {
	c.reads++
	return 0, io.EOF
}

func (c *readTrackingConn) Write(b []byte) (int, error) { return len(b), nil }
func (c *readTrackingConn) Close() error {
	c.closed = true
	return nil
}
func (c *readTrackingConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *readTrackingConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *readTrackingConn) SetDeadline(time.Time) error      { return nil }
func (c *readTrackingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *readTrackingConn) SetWriteDeadline(time.Time) error { return nil }

type bufferConn struct {
	bytes.Buffer
}

func (c *bufferConn) Close() error                     { return nil }
func (c *bufferConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *bufferConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *bufferConn) SetDeadline(time.Time) error      { return nil }
func (c *bufferConn) SetReadDeadline(time.Time) error  { return nil }
func (c *bufferConn) SetWriteDeadline(time.Time) error { return nil }

var _ net.Error = timeoutError{}
var _ net.Conn = (*bufferConn)(nil)
var _ net.Conn = (*readTrackingConn)(nil)
var _ net.Conn = (*timeoutConn)(nil)
var _ = errors.New
var _ = tls.VersionTLS12
