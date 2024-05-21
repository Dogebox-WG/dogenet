package msg

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// Dogecoin magic bytes for the mainnet
const MagicBytes = 0xc0c0c0c0
const MaxMsgSize = 0x2000000 // 32MB

// https://en.bitcoin.it/wiki/Protocol_documentation#version
type MessageHeader struct {
	Magic    uint32
	Command  string
	Length   uint32
	Checksum [4]byte
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

func DecodeHeader(buf [24]byte) (hdr MessageHeader) {
	hdr.Magic = binary.LittleEndian.Uint32(buf[:4])
	hdr.Command = string(bytes.TrimRight(buf[4:16], "\x00"))
	hdr.Length = binary.LittleEndian.Uint32(buf[16:20])
	copy(hdr.Checksum[:], buf[20:24])
	return
}

func ReadMessage(reader *bufio.Reader) (cmd string, payload []byte, err error) {
	// Read the message header
	buf := [24]byte{}
	n, err := io.ReadFull(reader, buf[:])
	if err != nil {
		return "", nil, fmt.Errorf("short header: received %d bytes: %v", n, err)
	}
	// Decode the header
	hdr := DecodeHeader(buf)
	if hdr.Magic != MagicBytes {
		return "", nil, fmt.Errorf("so sad, invalid magic bytes: %08x", hdr.Magic)
	}
	// Read the message payload
	payload = make([]byte, hdr.Length)
	n, err = io.ReadFull(reader, payload)
	if err != nil {
		return "", nil, fmt.Errorf("short payload: received %d bytes: %v", n, err)
	}
	// Verify checksum
	hash := DoubleSHA256(payload)
	if !bytes.Equal(hdr.Checksum[:], hash[:4]) {
		return "", nil, fmt.Errorf("so sad, checksum mismatch: %v vs %v", hdr.Checksum, hash[:4])
	}
	return hdr.Command, payload, nil
}

func DoubleSHA256(data []byte) [32]byte {
	hash := sha256.Sum256(data)
	return sha256.Sum256(hash[:])
}
