package protocol

import (
	"encoding/binary"

	"github.com/dogeorg/dogenet/internal/codec"
)

var AddrTag = binary.LittleEndian.Uint32([]byte("Addr"))

// service bit-flags
const (
	ServiceCoreNode  = 1  // runs a core node on port 22556 (participates in validating blocks and relaying transactions)
	ServiceArchive   = 2  // node has entire block history (can request any block)
	ServiceReserved1 = 4  // reserved for future dogecoin enhancement
	ServiceReserved2 = 8  // reserved for future dogecoin enhancement
	ServiceSocial    = 16 // provides social channels (e.g. chat, post-board)
	ServiceShop      = 32 // provides a shop (e.g. shibeshop)
)

type AddressMsg struct {
	Time     int64  // [8] Current time when this message is signed
	Services uint64 // [8] Services bit flags
	Address  []byte // [16] network byte order (BE); IPv4-mapped IPv6 address
	Port     uint16 // [2] network byte order (BE)
	Identity []byte // [32] doge identity pubkey (optional)
}

func DecodeAddrMsg(payload []byte, version int32) (msg AddressMsg) {
	d := codec.Decode(payload)
	msg.Time = d.Int64le()
	msg.Services = d.UInt64le()
	msg.Address = d.Bytes(16)
	msg.Port = d.UInt16be()
	return
}

func EncodeAddrMsg(msg AddressMsg) []byte {
	e := codec.Encode(34)
	e.Int64le(msg.Time)
	e.UInt64le(msg.Services)
	e.Bytes(msg.Address)
	e.UInt16be(msg.Port)
	return e.Result()
}
