package msg

import (
	"encoding/hex"
	"fmt"

	"code.dogecoin.org/gossip/codec"
)

type InvType uint32

const (
	InvError                InvType = 0          // ERROR
	InvTx                   InvType = 1          // MSG_TX: hash of transaction
	InvBlock                InvType = 2          // MSG_BLOCK: hash of block
	InvFilteredBLock        InvType = 3          // MSG_FILTERED_BLOCK: hash of block (BIP.37 reply merkleblock)
	InvCmpctBlock           InvType = 4          // MSG_CMPCT_BLOCK: hash of block (BIP.152 reply cmpctblock)
	InvWitnessTx            InvType = 0x40000001 // MSG_WITNESS_TX: hash of transaction with witness data (BIP.144)
	InvWitnessBlock         InvType = 0x40000002 // MSG_WITNESS_BLOCK: hash of block with witness data (BIP.144)
	InvFilteredWitnessBlock InvType = 0x40000003 //	MSG_FILTERED_WITNESS_BLOCK: hash of block with witness data (BIP.144 reply merkleblock)
)

type InvMsg struct {
	InvList []InvVector
}

func DecodeInvMsg(payload []byte) (msg InvMsg) {
	d := codec.Decode(payload)
	count := d.VarUInt()
	for i := uint64(0); i < count; i++ {
		var inv InvVector
		inv.Type = InvType(d.UInt32le())
		inv.Hash = d.Bytes(32)
		msg.InvList = append(msg.InvList, inv)
	}
	return
}

func EncodeInvMsg(msg InvMsg) []byte {
	e := codec.Encode(5 + 36*len(msg.InvList))
	e.VarUInt(uint64(len(msg.InvList)))
	for _, inv := range msg.InvList {
		e.UInt32le(uint32(inv.Type))
		e.Bytes(inv.Hash)
	}
	return e.Result()
}

type InvVector struct {
	Type InvType
	Hash []byte // hash of tx/block (32 bytes)
}

func (i *InvVector) String() string {
	return fmt.Sprintf("{%s %s}", InvTypeString(i.Type), hex.EncodeToString(i.Hash))
}

func DecodeInvVector(payload []byte) (msg InvVector) {
	d := codec.Decode(payload)
	msg.Type = InvType(d.UInt32le())
	msg.Hash = d.Bytes(32)
	return
}

func EncodeInvVector(msg InvVector) []byte {
	e := codec.Encode(36)
	e.UInt32le(uint32(msg.Type))
	e.Bytes(msg.Hash)
	return e.Result()
}

func InvTypeString(t InvType) string {
	switch t {
	case InvError:
		return "error"
	case InvTx:
		return "tx"
	case InvBlock:
		return "block"
	case InvFilteredBLock:
		return "filtered-block"
	case InvCmpctBlock:
		return "cmpct-block"
	case InvWitnessTx:
		return "witness-tx"
	case InvWitnessBlock:
		return "witness-block"
	case InvFilteredWitnessBlock:
		return "filtered-witness-block"
	}
	return "unknown"
}
