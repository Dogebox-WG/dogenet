package protocol

import (
	"encoding/binary"

	"github.com/dogeorg/dogenet/internal/codec"
)

var ChirpTag = binary.LittleEndian.Uint32([]byte("Chrp"))

type IdentityMsg struct { // 190+1584+104 = 1878
	Time    uint32 // [4] Current time when this message is signed (use to detect changes) (seconds since 2020)
	Name    string // [30] display name
	Bio     string // [120] short biography
	Lat     int16  // [2] WGS84 +/- 90 degrees, 60 seconds + 6ths (nearest 305m)
	Long    int16  // [2] WGS84 +/- 180 degrees, 60 seconds + 3rds (nearest 610m)
	Country string // [2] ISO 3166-1 alpha-2 code (optional)
	City    string // [30] city name (optional)
	Icon    []byte // [1584] 48x48 compressed (see dogeicon.go)
}

func DecodeIdentityMsg(payload []byte) (msg IdentityMsg) {
	d := codec.Decode(payload)
	msg.Time = d.UInt32le()
	msg.Name = d.VarString()
	msg.Bio = d.VarString()
	msg.Lat = int16(d.UInt16le())
	msg.Long = int16(d.UInt16le())
	msg.Country = d.PadString(2)
	msg.City = d.VarString()
	msg.Icon = d.Bytes(1584)
	return
}

func EncodeIdentityMsg(msg IdentityMsg) []byte {
	if len(msg.Name) > 30 {
		panic("Invalid identity: name longer than 30")
	}
	if len(msg.Bio) > 120 {
		panic("Invalid identity: bio longer than 120")
	}
	if len(msg.City) > 30 {
		panic("Invalid identity: city longer than 30")
	}
	if len(msg.Icon) != 1584 {
		panic("Invalid identity: icon size not 1584")
	}
	e := codec.Encode(8 + 33 + 129 + 1584)
	e.UInt32le(msg.Time)
	e.VarString(msg.Name)
	e.VarString(msg.Bio)
	e.UInt16le(uint16(msg.Lat))
	e.UInt16le(uint16(msg.Long))
	e.PadString(2, msg.Country)
	e.VarString(msg.City)
	e.Bytes(msg.Icon)
	return e.Result()
}
