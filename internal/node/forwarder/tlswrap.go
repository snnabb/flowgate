package forwarder

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

// GenerateSelfSignedCert generates an in-memory ECDSA P-256 self-signed certificate.
func GenerateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"FlowGate"}},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}

// NewTLSListener wraps an existing net.Listener with TLS using a self-signed certificate.
func NewTLSListener(inner net.Listener) (net.Listener, error) {
	cert, err := GenerateSelfSignedCert()
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return tls.NewListener(inner, tlsCfg), nil
}

// TLSDial connects to addr over TLS with the specified SNI.
func TLSDial(addr string, timeout time.Duration, sni string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	tlsCfg := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: sni == "", // skip verify when no SNI is set
		MinVersion:         tls.VersionTLS12,
	}

	return tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
}
