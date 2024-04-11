package msg

import (
	"encoding/binary"
)

// Decode

type Decoder struct {
	buf []byte
	pos uint64
}

func Decode(b []byte) *Decoder {
	return &Decoder{buf: b, pos: 0}
}

func (d *Decoder) bytes(num uint64) []byte {
	p := d.pos
	d.pos += num
	return d.buf[p : p+num]
}

func (d *Decoder) bool() bool {
	v := d.buf[d.pos]
	d.pos += 1
	if v != 0 {
		return true
	}
	return false
}

func (d *Decoder) uint8() uint8 {
	p := d.pos
	d.pos += 1
	return d.buf[p]
}

func (d *Decoder) uint16le() uint16 {
	p := d.pos
	d.pos += 2
	return binary.LittleEndian.Uint16(d.buf[p : p+2])
}

func (d *Decoder) uint16be() uint16 {
	p := d.pos
	d.pos += 2
	return binary.BigEndian.Uint16(d.buf[p : p+2])
}

func (d *Decoder) uint32le() uint32 {
	p := d.pos
	d.pos += 4
	return binary.LittleEndian.Uint32(d.buf[p : p+4])
}

func (d *Decoder) uint64le() uint64 {
	p := d.pos
	d.pos += 8
	return binary.LittleEndian.Uint64(d.buf[p : p+8])
}

func (d *Decoder) var_uint() uint64 {
	val := d.buf[d.pos]
	d.pos += 1
	if val < 253 {
		return uint64(val)
	}
	if val == 253 {
		return uint64(d.uint16le())
	}
	if val == 254 {
		return uint64(d.uint32le())
	}
	return d.uint64le()
}

func (d *Decoder) var_string() string {
	len := d.var_uint()
	data := d.bytes(len)
	return string(data)
}

func (d *Decoder) pad_string(size uint64) string {
	data := d.bytes(size)
	if size > 0 {
		end := size - 1
		for data[end] == 0 && end > 0 {
			end--
		}
		return string(data[:end+1])
	}
	return ""
}

// Encode

type Encoder struct {
	buf []byte
}

func Encode(size_hint int) *Encoder {
	return &Encoder{buf: make([]byte, 0, size_hint)}
}

func (e *Encoder) bytes(b []byte) {
	e.buf = append(e.buf, b...)
}

func (e *Encoder) bool(b bool) {
	var v byte = 0
	if b {
		v = 1
	}
	e.buf = append(e.buf, v)
}

func (e *Encoder) uint8(v uint8) {
	e.buf = append(e.buf, v)
}

func (e *Encoder) uint16le(v uint16) {
	e.buf = binary.LittleEndian.AppendUint16(e.buf, v)
}

func (e *Encoder) uint16be(v uint16) {
	e.buf = binary.BigEndian.AppendUint16(e.buf, v)
}

func (e *Encoder) uint32le(v uint32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *Encoder) uint64le(v uint64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, v)
}

func (e *Encoder) var_uint(val uint64) {
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

func (e *Encoder) var_string(v string) {
	b := []byte(v)
	e.var_uint(uint64(len(b)))
	e.buf = append(e.buf, b...)
}

func (e *Encoder) pad_string(size uint64, v string) {
	e.buf = append(e.buf, []byte(v)...)
	used := uint64(len(v))
	for used < size {
		e.buf = append(e.buf, 0)
	}
}
