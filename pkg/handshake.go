package dogenet

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/dogeorg/dogenet/pkg/msg"
)

const GobFilePath = "netmap.gob"
const InitProtocolVersion = 209

// Current Core Node version
const CurrentProtocolVersion = 70015

// Our DogeMap Node services
const DogeMapServices = msg.NodeNetworkLimited

// PawVersionMessage creates the version message payload
func PawVersionMessage() []byte {
	version := msg.VersionMsg{
		Version:   CurrentProtocolVersion,
		Services:  DogeMapServices,
		Timestamp: time.Now().Unix(),
		RemoteAddr: msg.NetAddr{
			Services: DogeMapServices,
			Address:  []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 14, 1, 84, 159},
			Port:     22556,
		},
		LocalAddr: msg.NetAddr{
			// NOTE: dogecoin nodes ignore these address fields.
			Services: DogeMapServices,
			Address:  []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
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
func RunService(localNode string) {
	// load the previously saved state.
	Map = NewNetMap()
	err := Map.ReadGob(GobFilePath)
	if err != nil {
		log.Println("Cannot read Gob file:", err)
		os.Exit(1) // XXX
		}
	}
*/

func ExpectVersion(reader *bufio.Reader) (msg.VersionMsg, error) {
	for {
		// Core Node implementation: if connection is inbound, send Version immediately.
		// This means we'll receive the Node's version before `verack` for our Version,
		// however this is undocumented, so other nodes might ack first.
		cmd, payload, err := msg.ReadMessage(reader)
		if err != nil {
			return msg.VersionMsg{}, fmt.Errorf("Error reading message: %v", err)
		}
		if cmd == "version" {
			return msg.DecodeVersion(payload), nil
		}
		if cmd == "verack" {
			// technically may receive verack before version.
			continue
		}
		if cmd == "reject" {
			re := msg.DecodeReject(payload)
			return msg.VersionMsg{}, fmt.Errorf("Reject: %s %s %s", re.CodeName(), re.Message, re.Reason)
		}
		return msg.VersionMsg{}, fmt.Errorf("Expected 'version' message from node, but received: %s", cmd)
	}
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

	nodeVer := version.Version // other node's version
	for {
		cmd, payload, err := msg.ReadMessage(reader)
		if err != nil {
			fmt.Println("Error reading message:", err)
			return
		}

		switch cmd {
		case "ping":
			fmt.Println("Ping received.")
			sendPong(conn, payload) // keep-alive

			// request a big list of known addresses (addr response)
			sendGetAddr(conn)
			fmt.Println("Sent getaddr.")

		case "reject":
			re := msg.DecodeReject(payload)
			fmt.Println("Reject:", re.CodeName(), re.Message, re.Reason)

		case "addr":
			addr := msg.DecodeAddrMsg(payload, nodeVer)
			fmt.Println("Addresses:")
			for _, a := range addr.AddrList {
				fmt.Println("• ", net.IP(a.Address), a.Port, "svc", a.Services, "ts", a.Time)
			}

		case "getheaders":
			ghdr := msg.DecodeGetHeaders(payload)
			fmt.Println("GetHeaders:", ghdr)

		case "inv":
			inv := msg.DecodeInvMsg(payload)
			fmt.Println("Inv:", inv)

		default:
			fmt.Printf("Command '%s' payload: %s\n", cmd, hex.EncodeToString(payload))
		}
	}

}

func sendPong(conn net.Conn, pingPayload []byte) {
	// reply with 'pong', same payload (nonce)
	_, err := conn.Write(msg.EncodeMessage("pong", pingPayload))
	if err != nil {
		fmt.Println("failed to send 'pong':", err)
		return
	}
}

func sendGetAddr(conn net.Conn) {
	_, err := conn.Write(msg.EncodeMessage("getaddr", []byte{}))
	if err != nil {
		fmt.Println("failed to send 'getaddr':", err)
		return
	}
}
