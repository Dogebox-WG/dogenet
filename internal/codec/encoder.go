package codec

import (
	"encoding/binary"
)

type Encoder struct {
	buf []byte
}

func Encode(size_hint int) *Encoder {
	return &Encoder{buf: make([]byte, 0, size_hint)}
}

// Get the encoded array of bytes
func (e *Encoder) Result() []byte {
	return e.buf
}

func (e *Encoder) Bytes(b []byte) {
	e.buf = append(e.buf, b...)
}

func (e *Encoder) Bool(b bool) {
	var v byte = 0
	if b {
		v = 1
	}
	e.buf = append(e.buf, v)
}

func (e *Encoder) UInt8(v uint8) {
	e.buf = append(e.buf, v)
}

func (e *Encoder) UInt16le(v uint16) {
	e.buf = binary.LittleEndian.AppendUint16(e.buf, v)
}

func (e *Encoder) UInt16be(v uint16) {
	e.buf = binary.BigEndian.AppendUint16(e.buf, v)
}

func (e *Encoder) UInt32le(v uint32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *Encoder) UInt64le(v uint64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, v)
}

func (e *Encoder) Int64le(v int64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, uint64(v))
}

func (e *Encoder) VarUInt(val uint64) {
	if val < 0xFD {
		e.buf = append(e.buf, byte(val))
	} else if val <= 0xFFFF {
		e.buf = append(e.buf, 0xFD)
		e.buf = binary.LittleEndian.AppendUint16(e.buf, uint16(val))
	} else if val <= 0xFFFFFFFF {
		e.buf = append(e.buf, 0xFE)
		e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(val))
	} else {
		e.buf = append(e.buf, 0xFF)
		e.buf = binary.LittleEndian.AppendUint64(e.buf, val)
	}
}

func (e *Encoder) VarString(v string) {
	b := []byte(v)
	e.VarUInt(uint64(len(b)))
	e.buf = append(e.buf, b...)
}

func (e *Encoder) PadString(size uint64, v string) {
	bytes := []byte(v)
	e.buf = append(e.buf, bytes[0:size]...)
	used := uint64(len(v))
	for used < size {
		e.buf = append(e.buf, 0)
	}
}
