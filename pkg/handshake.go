package dogenet

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/dogeorg/dogenet/pkg/msg"
)

// Initial protocol version for version/verack negotiation
const InitProtocolVersion = 209

// Current Core Node version
const CurrentProtocolVersion = 70015

// Our DogeMap Node services
const DogeMapServices = 1

// PawVersionMessage creates the version message payload
func PawVersionMessage() []byte {
	version := msg.VersionMessage{
		Version:   CurrentProtocolVersion,
		Services:  DogeMapServices,
		Timestamp: time.Now().Unix(),
		RemoteAddr: msg.NetAddr{
			Services: DogeMapServices,
			Address:  [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 14, 1, 84, 159},
			Port:     22556,
		},
		LocalAddr: msg.NetAddr{
			// NOTE: dogecoin nodes ignore these address fields.
			Services: DogeMapServices,
			Address:  [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			Port:     0,
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

func ExpectVersion(reader *bufio.Reader) (msg.VersionMessage, error) {
	// expect the version message from the node.
	cmd, payload, err := msg.ReadMessage(reader)
	if err != nil {
		return msg.VersionMessage{}, fmt.Errorf("Error reading message: %v", err)
	}
	if cmd == "version" {
		return msg.DecodeVersion(payload), nil
	}
	if cmd == "reject" {
		re := msg.DecodeReject(payload)
		return msg.VersionMessage{}, fmt.Errorf("Reject: %s %s %s", re.CodeName(), re.Message, re.Reason)
	}
	return msg.VersionMessage{}, fmt.Errorf("Expected 'version' message from node, but received: %s", cmd)
}

func ExpectVerAck(reader *bufio.Reader) error {
	// expect the verack message from the node.
	cmd, payload, err := msg.ReadMessage(reader)
	if err != nil {
		return fmt.Errorf("Error reading message: %v", err)
	}
	if cmd == "verack" {
		return nil
	}
	if cmd == "reject" {
		re := msg.DecodeReject(payload)
		return fmt.Errorf("Reject: %s %s %s", re.CodeName(), re.Message, re.Reason)
	}
	return fmt.Errorf("Expected 'verack' message, but received: %s", cmd)
}

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

	// Core Node logic: if connection is inbound (to Node) send the Version.
	// This means we'll receive the node's version immediately.

	// send our 'version' message
	_, err = conn.Write(msg.EncodeMessage("version", PawVersionMessage()))
	if err != nil {
		fmt.Println("Error sending version message:", err)
		return
	}

	fmt.Println("Sent 'version' message")

	// expect the version message from the node
	version, err := ExpectVersion(reader)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Received 'version':\n", version)

	// send 'verack' in response
	_, err = conn.Write(msg.EncodeMessage("verack", []byte{}))
	if err != nil {
		fmt.Println("failed to send 'verack':", err)
		return
	}

	fmt.Println("Sent 'verack'")

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
