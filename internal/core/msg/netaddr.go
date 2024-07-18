package msg

import "rad/gossip/codec"

// NetAddr represents the structure of a network address
type NetAddr struct {
	Time     uint32 // if version >= 31402; not present in version message
	Services uint64 // Services bit flags
	Address  []byte // [16] network byte order (BE); IPv4-mapped IPv6 address
	Port     uint16 // network byte order (BE)
}

const AddrTimeVersion = 31402 // Time field added to NetAddr.

// NB. pass version=0 in Version message.
func DecodeNetAddr(d *codec.Decoder, version int32) (a NetAddr) {
	if version >= AddrTimeVersion {
		a.Time = d.UInt32le()
	}
	a.Services = d.UInt64le()
	a.Address = d.Bytes(16)
	a.Port = d.UInt16be()
	return
}

// NB. pass version=0 in Version message.
func EncodeNetAddr(a NetAddr, e *codec.Encoder, version int32) {
	if version >= AddrTimeVersion {
		e.UInt32le(a.Time)
	}
	e.UInt64le(uint64(a.Services))
	e.Bytes(a.Address)
	e.UInt16be(a.Port)
}
