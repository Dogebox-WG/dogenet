package protocol

import (
	"encoding/binary"

	"github.com/dogeorg/dogenet/internal/codec"
)

var ChirpTag = binary.LittleEndian.Uint32([]byte("Chrp"))

type IdentityMsg struct {
	Time int64  // [8] Current time when this message is signed (use to detect changes)
	Name string // [Var] display name, max 32
	Bio  string // [Var] short biography, max 128
	/*
		1584 bytes Icon image: 48x48 YCbCr 4:2:0 compressed
		24x24 2x2 tiles; Y=6bit Cr=4bit Cb=4bit (clamp at limit); 2-bit tile-shape
		plane0 Y1Y2 (24*24*6*2/8=864) || plane1 CbCr (24*24*4*2/8=576) || plane2 shape (24*24*2/8=144) = 1584 (1.55K)
		plane0 Y1Y2 (24*24*6*2/8=864) || plane1 CbCr (24*24*6*2/8=864) || plane2 shape (24*24*2/8=144) = 1872 (1.83K) maybe?
		1/  \2  11  12   2 Y-samples per tile (6-bit samples packed into 12-bit plane)
		/2  1\  22  12   2-bit tile-shape as per diagram (0=/ 1=\ 2=H 3=V)
		lower bits are least significant; packed higher-to-lower as written
		tiles are ideally rendered as two equal-size halves with a Y-value for each half (modified by rendering effect)
	*/
	Icon []byte
	/*
		bits 1-0: interpolation: 0=flat 1=pixelated 2=bilinear 3=bicubic
		bits 4-2: effect: 0=none 1=venetian 2=vertical 3=diagonal-ltr 4=diagonal-rtl 5=dots 6=splats 7=scanline-fx
		lower bits are least significant
		flat: assign pixels the closest Y-value (average Y in tie-pixels, or tie-break consistently)
		pixelated: divide the tile into four equal-sized squares (average Y or tie-break off-axis diagonals?)
		bilinear and bicubic: in horizontal and vertical tiles: position Y at centres of point-pairs
		bilinear: calculate missing corner points bilinearly (cross-tile); then standard bilinear scaling
		bicubic: calculate missing corner points bicubically (cross-tile); then standard bicubic scaling (ideally)
	*/
	IconStyle byte
}

func DecodeIdentityMsg(payload []byte, version int32) (msg IdentityMsg) {
	d := codec.Decode(payload)
	msg.Time = d.Int64le()
	msg.Name = d.VarString()
	msg.Bio = d.VarString()
	msg.Icon = d.Bytes(1584)
	msg.IconStyle = d.UInt8()
	return
}

func EncodeIdentityMsg(msg IdentityMsg) []byte {
	if len(msg.Name) > 32 {

	}
	e := codec.Encode(34)
	e.Int64le(msg.Time)
	e.VarString(msg.Name)
	e.VarString(msg.Bio)
	e.UInt16be(msg.Port)
	return e.Result()
}
