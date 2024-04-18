package msg

type AddrMsg struct {
	AddrList []NetAddr
}

func DecodeAddrMsg(payload []byte, version int32) (msg AddrMsg) {
	d := Decode(payload)
	count := d.var_uint()
	for i := uint64(0); i < count; i++ {
		a := DecodeNetAddr(d, version)
		msg.AddrList = append(msg.AddrList, a)
	}
	return
}

func EncodeAddrMsg(msg AddrMsg, version int32) []byte {
	e := Encode(5 + 30*len(msg.AddrList)) // typically right (up to 1000 with Time field)
	e.var_uint(uint64(len(msg.AddrList)))
	for _, a := range msg.AddrList {
		EncodeNetAddr(a, e, version)
	}
	return e.buf
}
