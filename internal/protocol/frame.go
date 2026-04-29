package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// Message types for 1:1 WebSocket session.
const (
	CmdConnect   uint8 = 0x01
	CmdConnected uint8 = 0x02
	CmdData      uint8 = 0x03
	CmdClose     uint8 = 0x04

	AddrDomain uint8 = 0x01
	AddrIPv4   uint8 = 0x02
	AddrIPv6   uint8 = 0x03

	StatusOK              uint8 = 0x00
	StatusRefused         uint8 = 0x01
	StatusHostUnreachable uint8 = 0x02
	StatusTimeout         uint8 = 0x03
)

// ConnectRequest describes the target the client wants to reach.
type ConnectRequest struct {
	Addr     string
	Port     uint16
	AddrType uint8
	IsUDP    bool
}

func (r *ConnectRequest) Marshal() []byte {
	addr := []byte(r.Addr)
	buf := make([]byte, 1+1+1+len(addr)+2)
	i := 0
	if r.IsUDP {
		buf[i] = 0x02
	} else {
		buf[i] = 0x01
	}
	i++
	buf[i] = r.AddrType
	i++
	buf[i] = uint8(len(addr))
	i++
	copy(buf[i:], addr)
	i += len(addr)
	binary.BigEndian.PutUint16(buf[i:], r.Port)
	return buf
}

func ReadConnectRequest(data []byte) (*ConnectRequest, error) {
	if len(data) < 4 {
		return nil, errors.New("connect request too short")
	}
	r := &ConnectRequest{}
	r.IsUDP = data[0] == 0x02
	r.AddrType = data[1]
	addrLen := int(data[2])
	if len(data) < 3+addrLen+2 {
		return nil, errors.New("connect request truncated")
	}
	r.Addr = string(data[3 : 3+addrLen])
	r.Port = binary.BigEndian.Uint16(data[3+addrLen:])
	return r, nil
}

func (r *ConnectRequest) Network() string {
	if r.IsUDP {
		return "udp"
	}
	return "tcp"
}

func (r *ConnectRequest) Address() string {
	return net.JoinHostPort(r.Addr, fmt.Sprintf("%d", r.Port))
}

func AddrType(addr string) uint8 {
	ip := net.ParseIP(addr)
	if ip == nil {
		return AddrDomain
	}
	if ip.To4() != nil {
		return AddrIPv4
	}
	return AddrIPv6
}
