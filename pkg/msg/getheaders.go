package msg

type GetHeadersMsg struct {
	Version            uint32   // the protocol version
	BlockLocatorHashes [][]byte // [][32] block locator object; newest back to genesis block (dense to start, but then sparse)
	HashStop           []byte   // [32] hash of the last desired block header; set to zero to get as many blocks as possible (2000)
}

func DecodeGetHeaders(payload []byte) (msg GetHeadersMsg) {
	d := Decode(payload)
	msg.Version = d.uint32le()
	hashCount := d.var_uint()
	for i := uint64(0); i < hashCount; i++ {
		msg.BlockLocatorHashes = append(msg.BlockLocatorHashes, d.bytes(32))
	}
	msg.HashStop = d.bytes(32)
	return
}

func EncodeGetHeaders(msg GetHeadersMsg) []byte {
	e := Encode(4 + 5 + 32 + 32*len(msg.BlockLocatorHashes))
	e.uint32le(msg.Version)
	e.var_uint(uint64(len(msg.BlockLocatorHashes)))
	for _, hash := range msg.BlockLocatorHashes {
		e.bytes(hash)
	}
	e.bytes(msg.HashStop)
	return e.buf
}
