package protocol

import (
	"encoding/binary"

	"github.com/dogeorg/dogenet/internal/codec"
)

var TagReject = Tag4CC(binary.BigEndian.Uint32([]byte("Rejc")))

type RejectCode uint8

const (
	// message decode:
	REJECT_SIZE      RejectCode = 0x01
	REJECT_SIGNATURE RejectCode = 0x02
	REJECT_TAG       RejectCode = 0x03
	REJECT_CHAN      RejectCode = 0x04
	REJECT_OBSOLETE  RejectCode = 0x05
	// message handling:
	REJECT_LIMIT        RejectCode = 0x10
	REJECT_MALFORMED    RejectCode = 0x11
	REJECT_NOTPERMITTED RejectCode = 0x12
	// transactions:
	REJECT_DUPLICATE       RejectCode = 0x40
	REJECT_NONSTANDARD     RejectCode = 0x41
	REJECT_DUST            RejectCode = 0x42
	REJECT_INSUFFICIENTFEE RejectCode = 0x43
)

type RejectMsg struct {
	Code   RejectCode // uint8
	Reason string     // var-len {utf8}
	Data   []byte     // {bytes}  (transaction hash)
}

func (m *RejectMsg) CodeName() string {
	switch m.Code {
	case REJECT_SIZE:
		return "message too large"
	case REJECT_SIGNATURE:
		return "bad signature"
	case REJECT_TAG:
		return "unrecognised tag"
	case REJECT_CHAN:
		return "unrecognised channel"
	case REJECT_OBSOLETE:
		return "obsolete message"
	case REJECT_MALFORMED:
		return "malformed message"
	case REJECT_LIMIT:
		return "limit exceeded"
	case REJECT_NOTPERMITTED:
		return "not permitted"
	case REJECT_DUPLICATE:
		return "duplicate transaction"
	case REJECT_NONSTANDARD:
		return "nonstandard transaction"
	case REJECT_DUST:
		return "below dust limit"
	case REJECT_INSUFFICIENTFEE:
		return "insufficient fee"
	default:
		return "unknown"
	}
}

func EncodeReject(code RejectCode, reason string, data []byte) []byte {
	e := codec.Encode(100)
	e.UInt8(uint8(code))
	e.VarString(reason)
	e.Bytes(data)
	return e.Result()
}

func DecodeReject(msg []byte) (rej RejectMsg) {
	d := codec.Decode(msg)
	rej.Code = RejectCode(d.UInt8())
	rej.Reason = d.VarString()
	rej.Data = d.Rest()
	return
}
