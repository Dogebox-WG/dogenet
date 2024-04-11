package msg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

// Dogecoin magic bytes for the mainnet
const MagicBytes = 0xc0c0c0c0

type MessageHeader struct {
	Magic    uint32
	Command  string
	Length   uint32
	Checksum [4]byte
}

func DecodeHeader(buf [24]byte) (hdr MessageHeader) {
	hdr.Magic = binary.LittleEndian.Uint32(buf[:4])
	hdr.Command = string(bytes.TrimRight(buf[4:16], "\x00"))
	hdr.Length = binary.LittleEndian.Uint32(buf[16:20])
	copy(hdr.Checksum[:], buf[20:24])
	return
}

func DoubleSHA256(data []byte) [32]byte {
	hash := sha256.Sum256(data)
	return sha256.Sum256(hash[:])
}

func EncodeMessage(cmd string, payload []byte) []byte {
	msg := make([]byte, 24+len(payload))
	binary.LittleEndian.PutUint32(msg[:4], MagicBytes)
	copy(msg[4:16], cmd)
	binary.LittleEndian.PutUint32(msg[16:20], uint32(len(payload)))
	hash := DoubleSHA256(payload)
	copy(msg[20:24], hash[:4])
	copy(msg[24:], payload)
	return msg
}
