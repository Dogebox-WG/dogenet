package codec

import (
	"encoding/binary"
	"fmt"
	"math"
)

type Decoder struct {
	buf []byte
	pos int // current (unread) byte position
}

func Decode(b []byte) *Decoder {
	return &Decoder{buf: b}
}

func (d *Decoder) Has(n int) bool {
	return d.pos+n <= len(d.buf)
}

func (d *Decoder) Bytes(num int) []byte {
	p := d.pos
	d.pos += num
	return d.buf[p : p+num]
}

// All remaining bytes in the buffer
func (d *Decoder) Rest() []byte {
	p := d.pos
	d.pos = len(d.buf)
	return d.buf[p:]
}

func (d *Decoder) Bool() bool {
	v := d.buf[d.pos]
	d.pos += 1
	return v != 0
}

func (d *Decoder) UInt8() uint8 {
	p := d.pos
	d.pos += 1
	return d.buf[p]
}

func (d *Decoder) UInt16le() uint16 {
	p := d.pos
	d.pos += 2
	return binary.LittleEndian.Uint16(d.buf[p : p+2])
}

func (d *Decoder) UInt16be() uint16 {
	p := d.pos
	d.pos += 2
	return binary.BigEndian.Uint16(d.buf[p : p+2])
}

func (d *Decoder) UInt32le() uint32 {
	p := d.pos
	d.pos += 4
	return binary.LittleEndian.Uint32(d.buf[p : p+4])
}

func (d *Decoder) UInt64le() uint64 {
	p := d.pos
	d.pos += 8
	return binary.LittleEndian.Uint64(d.buf[p : p+8])
}

func (d *Decoder) VarUInt() uint64 {
	val := d.buf[d.pos]
	d.pos += 1
	if val < 253 {
		return uint64(val)
	}
	if val == 253 {
		return uint64(d.UInt16le())
	}
	if val == 254 {
		return uint64(d.UInt32le())
	}
	return d.UInt64le()
}

func (d *Decoder) VarString() string {
	len := d.VarUInt()
	if len > math.MaxInt {
		panic(fmt.Sprintf("decoded string length too long (greater than max-int): %v", len))
	}
	data := d.Bytes(int(len))
	return string(data)
}

func (d *Decoder) PadString(size int) string {
	data := d.Bytes(size)
	if size > 0 {
		end := size - 1
		for data[end] == 0 && end > 0 {
			end--
		}
		return string(data[:end+1])
	}
	return ""
}
