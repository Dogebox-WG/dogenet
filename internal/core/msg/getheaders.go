package msg

import "rad/gossip/codec"

type GetHeadersMsg struct {
	Version            uint32   // the protocol version
	BlockLocatorHashes [][]byte // [][32] block locator object; newest back to genesis block (dense to start, but then sparse)
	HashStop           []byte   // [32] hash of the last desired block header; set to zero to get as many blocks as possible (2000)
}

func DecodeGetHeaders(payload []byte) (msg GetHeadersMsg) {
	d := codec.Decode(payload)
	msg.Version = d.UInt32le()
	hashCount := d.VarUInt()
	for i := uint64(0); i < hashCount; i++ {
		msg.BlockLocatorHashes = append(msg.BlockLocatorHashes, d.Bytes(32))
	}
	msg.HashStop = d.Bytes(32)
	return
}

func EncodeGetHeaders(msg GetHeadersMsg) []byte {
	e := codec.Encode(4 + 5 + 32 + 32*len(msg.BlockLocatorHashes))
	e.UInt32le(msg.Version)
	e.VarUInt(uint64(len(msg.BlockLocatorHashes)))
	for _, hash := range msg.BlockLocatorHashes {
		e.Bytes(hash)
	}
	e.Bytes(msg.HashStop)
	return e.Result()
}
