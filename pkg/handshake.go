package dogenet

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"time"
)

// Dogecoin magic bytes for the mainnet
const magicBytes = 0xc0c0c0c0

// VersionMessage represents the structure of the version message
type VersionMessage struct {
	Version    int32
	Services   uint64
	Timestamp  int64
	RemoteAddr NetAddr
	LocalAddr  NetAddr
	Agent      string
	Nonce      uint64
	Height     int32
	Relay      bool
}

// NetAddr represents the structure of a network address
type NetAddr struct {
	Services uint64
	IP       [4]byte
	Port     uint16
}

// preparePacket creates a version message packet
func preparePacket(cmd string, payload []byte) []byte {
	msg := make([]byte, 24+len(payload))
	var checksum [4]byte

	// Magic Bytes
	binary.LittleEndian.PutUint32(msg[:4], magicBytes)

	// Command
	copy(msg[4:16], cmd)

	// Payload length
	binary.LittleEndian.PutUint32(msg[16:20], uint32(len(payload)))

	// checksum = sha256(sha256(payload))
	hash := sha256.Sum256(payload)
	copy(checksum[:], hash[:4])

	// Checksum
	copy(msg[20:24], checksum[:])

	// Payload
	copy(msg[24:], payload)

	return msg
}

// decodePacket decodes the received packet
func decodePacket(packet []byte) (cmd string, payload []byte, err error) {
	fmt.Println("Much Received Packet:", hex.EncodeToString(packet))

	if len(packet) < 24 {
		return "", nil, fmt.Errorf("So sad, packet length is less than 24 bytes")
	}

	// Verify magic bytes
	if binary.LittleEndian.Uint32(packet[:4]) != magicBytes {
		return "", nil, fmt.Errorf("So sad, invalid magic bytes")
	}

	// Get command
	cmd = string(bytes.TrimRight(packet[4:16], "\x00"))

	// Get payload length
	payloadLen := binary.LittleEndian.Uint32(packet[16:20])

	// Get checksum
	checksum := packet[20:24]

	// Verify checksum
	hash := sha256.Sum256(packet[24 : 24+payloadLen])
	hash = sha256.Sum256(hash[:])
	if !bytes.Equal(checksum, hash[:4]) {
		return "", nil, fmt.Errorf("So sad, checksum mismatch")
	}

	// Get payload
	payload = packet[24 : 24+payloadLen]

	return cmd, payload, nil
}

// PawVersionMessage creates the version message payload
func PawVersionMessage() []byte {
	version := VersionMessage{
		Version:   70015,
		Services:  1,
		Timestamp: time.Now().Unix(),
		RemoteAddr: NetAddr{
			Services: 1,
			IP:       [4]byte{127, 0, 0, 1},
			Port:     22556,
		},
		LocalAddr: NetAddr{
			Services: 1,
			IP:       [4]byte{127, 0, 0, 1},
			Port:     22556,
		},
		Agent:  "/DogeBox: DogeMap Service/",
		Nonce:  1,
		Height: 0,
		Relay:  false,
	}

	encoded := muchEncodeVersionMessage(version)
	return encoded
}

// muchEncodeVersionMessage encodes the version message
func muchEncodeVersionMessage(version VersionMessage) []byte {
	buf := make([]byte, 0, 86)

	binary.LittleEndian.PutUint32(buf[:4], uint32(version.Version))
	binary.LittleEndian.PutUint64(buf[4:12], version.Services)
	binary.LittleEndian.PutUint64(buf[12:20], uint64(version.Timestamp))

	copy(buf[20:46], soEncodeNetAddr(version.RemoteAddr))
	copy(buf[46:72], soEncodeNetAddr(version.LocalAddr))

	binary.LittleEndian.PutUint64(buf[72:80], version.Nonce)

	userAgent := []byte(version.Agent)
	buf = append(buf, byte(len(userAgent)))
	buf = append(buf, userAgent...)

	binary.LittleEndian.PutUint32(buf[80:84], uint32(version.Height))

	if version.Relay {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}

	return buf
}

// soEncodeNetAddr encodes the network address
func soEncodeNetAddr(addr NetAddr) []byte {
	buf := make([]byte, 26)

	binary.LittleEndian.PutUint64(buf[:8], addr.Services)
	copy(buf[8:12], addr.IP[:])
	binary.BigEndian.PutUint16(buf[20:22], addr.Port)

	return buf
}

/*
	// Query seed nodes
	seedNodes := []string{"seed.multidoge.org"}
	for _, node := range seedNodes {
		ips, err := net.LookupIP(node)
		if err != nil {
			fmt.Println("Error resolving DNS:", err)
			continue
		}
		for _, ip := range ips {
			testNode(db, ip.String())
		}
	}
*/

// testNode connects to a Dogecoin node and tests it
func TestNode(nodeIP string) {
	fmt.Println("Testing Dogecoin node:", nodeIP)
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:22556", nodeIP))
	if err != nil {
		fmt.Println("Error connecting to Dogecoin node:", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(preparePacket("version", PawVersionMessage()))
	if err != nil {
		fmt.Println("Error sending version message:", err)
		return
	}
	fmt.Println("Version message sent")

	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	cmd, payload, err := decodePacket(response[:n])
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return
	}

	fmt.Println("Response Command:", cmd)
	fmt.Println("Response Payload:", hex.EncodeToString(payload))
}
