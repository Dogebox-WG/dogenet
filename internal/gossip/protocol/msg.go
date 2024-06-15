package protocol

import (
	"bufio"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
)

const MaxMsgSize = 0x1000080 // 16MB (block size is 1MB; 16x=16MB + 128 header 0x80)

// DogeNet magic bytes?
// var NetworkID = binary.LittleEndian.Uint32([]byte("DNet"))

type RawPriv = []byte             // [32]seed (random)
type PrivKey = ed25519.PrivateKey // [32]privkey then [32]pubkey
type PubKey = ed25519.PublicKey   // [32]pubkey

func NewPrivKey(priv []byte) (pk PrivKey) {
	if len(priv) != 32 {
		panic("PrivKey: malformed private key (not 32 bytes)")
	}
	pk = ed25519.NewKeyFromSeed(priv)
	return
}

type MessageHeader struct { // 104 bytes
	Tag       uint32 // big-endian [4]byte code
	Length    uint32
	PubKey    []byte // [32]byte
	Signature []byte // [64]byte
}

// var accepted = map[uint32]string{
// 	binary.LittleEndian.Uint32([]byte("DNet")): "DNet",
// }

func EncodeMessage(tag uint32, privkey PrivKey, payload []byte) []byte {
	if len(payload) > MaxMsgSize {
		panic("EncodeMessage: message too large: " + strconv.Itoa(len(payload)))
	}
	msg := make([]byte, 104+len(payload))
	binary.LittleEndian.PutUint32(msg[0:4], tag)
	binary.LittleEndian.PutUint32(msg[4:8], uint32(len(payload)))
	copy(msg[8:40], privkey[32:])                     // 32 bytes: inline privkey.Public()
	copy(msg[40:104], ed25519.Sign(privkey, payload)) // 64 bytes
	copy(msg[104:], payload)
	return msg
}

func DecodeHeader(buf *[104]byte) (hdr MessageHeader) {
	hdr.Tag = binary.LittleEndian.Uint32(buf[0:4])
	hdr.Length = binary.LittleEndian.Uint32(buf[4:8])
	hdr.PubKey = buf[12:40]     // [32]byte
	hdr.Signature = buf[40:104] // [64]byte
	return
}

func ReadMessage(reader *bufio.Reader) (tag uint32, payload []byte, pubkey []byte, err error) {
	// Read the message header
	buf := [104]byte{}
	n, err := io.ReadFull(reader, buf[:])
	if err != nil {
		return 0, nil, []byte{}, fmt.Errorf("short header: received %d bytes: %v", n, err)
	}
	// Decode the header
	hdr := DecodeHeader(&buf)
	if hdr.Length > MaxMsgSize {
		return 0, nil, []byte{}, fmt.Errorf("message too large: [%s] size is %d bytes", TagToStr(hdr.Tag), hdr.Length)
	}
	// Read the message payload
	payload = make([]byte, hdr.Length)
	n, err = io.ReadFull(reader, payload)
	if err != nil {
		return 0, nil, []byte{}, fmt.Errorf("short payload: [%s] received %d of %d bytes: %v", TagToStr(hdr.Tag), n, hdr.Length, err)
	}
	// Verify signature
	if !ed25519.Verify(hdr.PubKey, payload, hdr.Signature) {
		return 0, nil, []byte{}, fmt.Errorf("incorrect signature: [%s] message", TagToStr(hdr.Tag))
	}
	return hdr.Tag, payload, hdr.PubKey, nil
}

func TagToStr(tag uint32) string {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], tag)
	return string(buf[:])
}

func TagToBytes(tag uint32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], tag)
	return buf[:]
}
