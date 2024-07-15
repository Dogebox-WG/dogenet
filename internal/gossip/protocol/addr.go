package protocol

import (
	"github.com/dogeorg/dogenet/internal/codec"
)

var TagAddress = makeTag4CC("Addr")

const AddrMsgSize = 56

type AddressMsg struct { // 56 + 4c + 6s
	Time    DogeTime // [4] Current Doge Epoch time when this message is signed
	Address []byte   // [16] network byte order (Big-Endian); IPv4-mapped IPv6 address
	Port    uint16   // [2] network byte order (Big-Endian)
	Owner   []byte   // [32] identity pubkey (zeroes if not present)
	// [1] number of channels
	Channels []Tag4CC // [4] per service (Chan)
	// [1] number of services
	Services []Service // [6] per service (ID + Port)
}

type Service struct {
	Tag  Tag4CC // [4] Service Tag (Big-Endian)
	Port uint16 // [2] TCP Port number (Big-Endian)
	Data string // [1+] Service Data (optional)
}

func (msg AddressMsg) Encode() []byte {
	if len(msg.Services) > 8192 {
		panic("Invalid AddrMsg: more than 8192 services")
	}
	if len(msg.Owner) != 32 {
		panic("Invalid Owner: must be 32 bytes")
	}
	e := codec.Encode(AddrMsgSize)
	e.UInt32le(uint32(msg.Time))
	e.Bytes(msg.Address)
	e.UInt16be(msg.Port)
	e.Bytes(msg.Owner)
	e.VarUInt(uint64(len(msg.Channels)))
	for n := 0; n < len(msg.Channels); n++ {
		e.UInt32be(uint32(msg.Channels[n]))
	}
	e.VarUInt(uint64(len(msg.Services)))
	for n := 0; n < len(msg.Services); n++ {
		e.UInt32be(uint32(msg.Services[n].Tag))
		e.UInt16be(msg.Services[n].Port)
		e.VarString(msg.Services[n].Data)
	}
	return e.Result()
}

func DecodeAddrMsg(payload []byte, version int32) (msg AddressMsg) {
	d := codec.Decode(payload)
	msg.Time = DogeTime(d.UInt32le())
	msg.Address = d.Bytes(16)
	msg.Port = d.UInt16be()
	msg.Owner = d.Bytes(32)
	// decode channels
	nchannel := d.VarUInt()
	if nchannel > 8192 {
		panic("Invalid AddrMsg: more than 8192 services")
	}
	msg.Channels = make([]Tag4CC, nchannel)
	for n := 0; n < int(nchannel); n++ {
		msg.Channels[n] = Tag4CC(d.UInt32be())
	}
	// decode services
	nservice := d.VarUInt()
	if nservice > 8192 {
		panic("Invalid AddrMsg: more than 8192 services")
	}
	msg.Services = make([]Service, nservice)
	for n := 0; n < int(nservice); n++ {
		msg.Services[n].Tag = Tag4CC(d.UInt32be())
		msg.Services[n].Port = d.UInt16be()
		msg.Services[n].Data = d.VarString()
	}
	return
}
