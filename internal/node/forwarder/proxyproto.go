package forwarder

import (
	"encoding/binary"
	"fmt"
	"net"
)

// PROXY protocol v2 signature (12 bytes)
var proxyV2Sig = [12]byte{
	0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A,
}

// WriteProxyV1 writes a PROXY protocol v1 header.
// Format: "PROXY TCP4 srcIP dstIP srcPort dstPort\r\n"
func WriteProxyV1(dst net.Conn, clientAddr, localAddr net.Addr) error {
	srcIP, srcPort := addrIPPort(clientAddr)
	dstIP, dstPort := addrIPPort(localAddr)

	proto := "TCP4"
	if srcIP.To4() == nil {
		proto = "TCP6"
	}

	header := fmt.Sprintf("PROXY %s %s %s %d %d\r\n", proto, srcIP, dstIP, srcPort, dstPort)
	_, err := dst.Write([]byte(header))
	return err
}

// WriteProxyV2 writes a PROXY protocol v2 header (binary format).
func WriteProxyV2(dst net.Conn, clientAddr, localAddr net.Addr) error {
	srcIP, srcPort := addrIPPort(clientAddr)
	dstIP, dstPort := addrIPPort(localAddr)

	var buf []byte
	buf = append(buf, proxyV2Sig[:]...)

	// Version (2) and command (PROXY = 0x01) => 0x21
	buf = append(buf, 0x21)

	ip4Src := srcIP.To4()
	ip4Dst := dstIP.To4()

	if ip4Src != nil && ip4Dst != nil {
		// AF_INET + STREAM => 0x11
		buf = append(buf, 0x11)
		// Address length: 4+4+2+2 = 12
		buf = append(buf, 0x00, 0x0C)
		buf = append(buf, ip4Src...)
		buf = append(buf, ip4Dst...)
		portBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(portBuf, uint16(srcPort))
		buf = append(buf, portBuf...)
		binary.BigEndian.PutUint16(portBuf, uint16(dstPort))
		buf = append(buf, portBuf...)
	} else {
		// AF_INET6 + STREAM => 0x21
		buf = append(buf, 0x21)
		// Address length: 16+16+2+2 = 36
		buf = append(buf, 0x00, 0x24)
		buf = append(buf, srcIP.To16()...)
		buf = append(buf, dstIP.To16()...)
		portBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(portBuf, uint16(srcPort))
		buf = append(buf, portBuf...)
		binary.BigEndian.PutUint16(portBuf, uint16(dstPort))
		buf = append(buf, portBuf...)
	}

	_, err := dst.Write(buf)
	return err
}

func addrIPPort(addr net.Addr) (net.IP, int) {
	switch a := addr.(type) {
	case *net.TCPAddr:
		return a.IP, a.Port
	case *net.UDPAddr:
		return a.IP, a.Port
	default:
		return net.IPv4zero, 0
	}
}
