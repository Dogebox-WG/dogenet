package spec

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
)

// Address is an IP:Port combination.
type Address struct {
	Host net.IP
	Port uint16
}

func (a Address) String() string {
	return net.JoinHostPort(a.Host.String(), strconv.Itoa(int(a.Port)))
}

func (a Address) IsValid() bool {
	return a.Port != 0 && len(a.Host) >= 4
}

// NodeID creates a NodeID from a host:port pair.
func (a Address) NodeID() NodeID {
	var id NodeID
	id[0] = NodeIDAddress
	copy(id[1:17], a.Host)
	binary.BigEndian.PutUint16(id[17:], a.Port)
	return id
}

func ParseAddress(hostport string) (Address, error) {
	hosts, ports, err := net.SplitHostPort(hostport)
	if err != nil {
		return Address{}, err
	}
	host := net.ParseIP(hosts)
	if host == nil {
		return Address{}, errors.New("bad ip")
	}
	port, err := strconv.Atoi(ports)
	if err != nil {
		return Address{}, err
	}
	if port < 0 || port > 65535 {
		return Address{}, errors.New("range")
	}
	return Address{Host: host, Port: uint16(port)}, nil
}

type PubKey []byte  // Public Key 32 bytes
type PrivKey []byte // Private Key 32 bytes (seed entropy)

func (key PubKey) String() string {
	return hex.EncodeToString(key[:])
}

// NodeID creates a NodeID from a public key.
func (key PubKey) NodeID() NodeID {
	var id NodeID
	id[0] = NodeIDPubKey
	copy(id[1:], key[:])
	return id
}

// NodeID is an Address (for Core Nodes) or 32-byte PubKey (for DogeBox nodes)
type NodeID [33]byte

const (
	NodeIDAddress byte = 1
	NodeIDPubKey  byte = 2
)

func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}
