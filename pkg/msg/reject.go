package msg

type RejectCode int

const (
	REJECT_MALFORMED       RejectCode = 0x01
	REJECT_INVALID         RejectCode = 0x10
	REJECT_OBSOLETE        RejectCode = 0x11
	REJECT_DUPLICATE       RejectCode = 0x12
	REJECT_NONSTANDARD     RejectCode = 0x40
	REJECT_DUST            RejectCode = 0x41
	REJECT_INSUFFICIENTFEE RejectCode = 0x42
	REJECT_CHECKPOINT      RejectCode = 0x43
)

type RejectMsg struct {
	Message string
	Code    RejectCode
	Reason  string
	Data    []byte
}

func (m *RejectMsg) CodeName() string {
	switch m.Code {
	case 0x01:
		return "malformed"
	case 0x10:
		return "invalid"
	case 0x11:
		return "obsolete"
	case 0x12:
		return "duplicate"
	case 0x40:
		return "nonstandard"
	case 0x41:
		return "dust"
	case 0x42:
		return "insufficient-fee"
	case 0x43:
		return "checkpoint"
	default:
		return "unknown"
	}
}

func DecodeReject(msg []byte) (rej RejectMsg) {
	d := Decode(msg)
	rej.Message = d.var_string()
	rej.Code = RejectCode(d.uint8())
	rej.Reason = d.var_string()
	rej.Data = d.buf[d.pos:]
	return
}
