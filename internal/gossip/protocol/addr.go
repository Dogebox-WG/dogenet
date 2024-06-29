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
	Time        uint32    // [4] Current time when this message is signed (seconds since 2020)
	ServiceBits uint64    // [8] Services bit flags
	Address     []byte    // [16] network byte order (BE); IPv4-mapped IPv6 address
	Port        uint16    // [2] network byte order (BE)
	Identity    []byte    // [32] doge identity pubkey (optional)
	Services    []Service // [6] per service (Code + Port)
}

type Service struct {
	Code uint32 // [4]ASCII gossip channel ID
	Port uint16 // [2]Port > 0 if service has its own TCP port
}

func DecodeAddrMsg(payload []byte, version int32) (msg AddressMsg) {
	d := codec.Decode(payload)
	msg.Time = d.UInt32le()
	msg.ServiceBits = d.UInt64le()
	msg.Address = d.Bytes(16)
	msg.Port = d.UInt16be()
	nservice := d.VarUInt()
	if nservice > 65535 {
		panic("Invalid AddrMsg: more than 65535 services")
	}
	msg.Services = make([]Service, nservice)
	for n := 0; n < int(nservice); n++ {
		msg.Services[n].Code = d.UInt32le()
		msg.Services[n].Port = d.UInt16be()
	}
	return
}

func EncodeAddrMsg(msg AddressMsg) []byte {
	if len(msg.Services) > 65535 {
		panic("Invalid AddrMsg: more than 65535 services")
	}
	e := codec.Encode(34)
	e.UInt32le(msg.Time)
	e.UInt64le(msg.ServiceBits)
	e.Bytes(msg.Address)
	e.UInt16be(msg.Port)
	e.VarUInt(uint64(len(msg.Services)))
	for n := 0; n < len(msg.Services); n++ {
		e.UInt32le(msg.Services[n].Code)
		e.UInt16be(msg.Services[n].Port)
	}
	return e.Result()
}
