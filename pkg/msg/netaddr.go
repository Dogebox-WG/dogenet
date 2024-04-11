package msg

// NetAddr represents the structure of a network address
type NetAddr struct {
	Time     uint32 // if version >= 31402; not present in version message
	Services uint64
	Address  [16]byte // network byte order (BE); IPv4-mapped IPv6 address
	Port     uint16   // network byte order (BE)
}

const AddrTimeVersion = 31402 // Time field added to NetAddr.

// NB. pass version=0 in Version message.
func DecodeNetAddr(d *Decoder, version int32) (a NetAddr) {
	if version >= AddrTimeVersion {
		a.Time = d.uint32le()
	}
	a.Services = d.uint64le()
	copy(a.Address[:], d.bytes(16))
	a.Port = d.uint16be()
	return
}

// NB. pass version=0 in Version message.
func EncodeNetAddr(a NetAddr, e *Encoder, version int32) {
	if version >= AddrTimeVersion {
		e.uint32le(a.Time)
	}
	e.uint64le(a.Services)
	e.bytes(a.Address[:])
	e.uint16be(a.Port)
	return
}
