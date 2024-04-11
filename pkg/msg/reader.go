package msg

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

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
