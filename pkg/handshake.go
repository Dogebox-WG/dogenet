package dogenet

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/dogeorg/dogenet/pkg/msg"
)

// PawVersionMessage creates the version message payload
func PawVersionMessage() []byte {
	version := msg.VersionMessage{
		Version:   70015,
		Services:  1,
		Timestamp: time.Now().Unix(),
		RemoteAddr: msg.NetAddr{
			Services: 1,
			Address:  [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1},
			Port:     22556,
		},
		LocalAddr: msg.NetAddr{
			Services: 1,
			Address:  [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1},
			Port:     22556,
		},
		Agent:  "/DogeBox: DogeMap Service/",
		Nonce:  23972479,
		Height: 0,
		Relay:  false,
	}
	return msg.EncodeVersion(version)
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
	reader := bufio.NewReader(conn)

	_, err = conn.Write(msg.EncodeMessage("version", PawVersionMessage()))
	if err != nil {
		fmt.Println("Error sending version message:", err)
		return
	}
	fmt.Println("Version message sent")

	for {
		cmd, payload, err := msg.ReadMessage(reader)
		if err != nil {
			fmt.Println("Error reading message:", err)
			return
		}

		fmt.Println("Response Command:", cmd)
		fmt.Println("Response Payload:", hex.EncodeToString(payload))

		if cmd == "reject" {
			re := msg.DecodeReject(payload)
			fmt.Println("Reject:", re.CodeName(), re.Message, re.Reason)
		}
	}
}
