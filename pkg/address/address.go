package address

import (
	"encoding/binary"
	"encoding/hex"

	"code.dogecoin.org/gossip/dnet"
)

type Address = dnet.Address
type PubKey = dnet.PubKey
type PrivKey = dnet.PrivKey

// NodeID is an Address (for Core Nodes) or 32-byte PubKey (for DogeBox nodes)
type NodeID [33]byte

const (
	NodeIDAddress byte = 1
	NodeIDPubKey  byte = 2
)

func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}

// NodeID creates a NodeID from a host:port pair.
func NodeIDFromAddress(a Address) NodeID {
	var id NodeID
	id[0] = NodeIDAddress
	copy(id[1:17], a.Host)
	binary.BigEndian.PutUint16(id[17:], a.Port)
	return id
}

// NodeID creates a NodeID from a public key.
func NodeIDFromKey(key PubKey) NodeID {
	var id NodeID
	id[0] = NodeIDPubKey
	copy(id[1:], key[:])
	return id
}

// BindTo binds to either a Unix socket or a TCP interface
type BindTo struct {
	Network string // "unix" or "tcp"
	Address string // unix file path, or <addr>:<port>
}
