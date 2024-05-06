package dogenet

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dogeorg/dogenet/pkg/msg"
)

const GobFilePath = "netmap.gob"

const ExpiryTime = time.Duration(2 * 24 * time.Hour)

var Map NetMap // persistent network map

// Current Core Node version
const CurrentProtocolVersion = 70015

// Minimum height accepted by other nodes
const MinimumBlockHeight = 700000

// Our DogeMap Node services
const DogeMapServices = 0

// PawVersionMessage creates the version message payload
func PawVersionMessage(remoteVersion int32) []byte {
	version := msg.VersionMsg{
		Version:   min(CurrentProtocolVersion, remoteVersion),
		Services:  DogeMapServices,
		Timestamp: time.Now().Unix(),
		RemoteAddr: msg.NetAddr{
			Services: DogeMapServices,
			Address:  []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 14, 1, 84, 159},
			Port:     22556,
		},
		LocalAddr: msg.NetAddr{
			Services: DogeMapServices,
			// NOTE: dogecoin nodes ignore these address fields.
			Address: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			Port:    0,
		},
		Agent:  "/DogeBox: DogeMap Service/",
		Nonce:  23972479,
		Height: MinimumBlockHeight,
		Relay:  false,
	}
	return msg.EncodeVersion(version)
}

type Service interface {
	Stop()
}

func RunService(localNode string) {
	// load the previously saved state.
	Map = NewNetMap()
	err := Map.ReadGob(GobFilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Println("Cannot read Gob file:", err)
		os.Exit(1)
	}

	// cancel context for shutdown.
	wg := &sync.WaitGroup{}
	services := []Service{}

	// periodically save the network map.
	saver := NewMapSaver(wg)
	services = append(services, saver)

	// stay connected to local node if specified.
	if localNode != "" {
		addr := net.ParseIP(localNode)
		if addr == nil {
			log.Println("Invalid ip address on command line:", localNode)
			os.Exit(1)
		}
		local := NewCollector(wg, net.JoinHostPort(localNode, "22556"), 0)
		services = append(services, local)
	}

	// start connecting to remote nodes.
	crawler := NewCollector(wg, "", 5*time.Minute)
	services = append(services, crawler)

	// start the web server.
	webserver := NewWebAPI(wg, "0.0.0.0", 8086)
	services = append(services, webserver)

	// Set up signal handling.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	// detect shutdown.
	<-signals
	fmt.Println("")
	fmt.Println("Shutdown requested via signal")
	for _, svc := range services {
		svc.Stop()
	}
	wg.Wait()
	fmt.Println("Writing the netmap database")
	Map.WriteGob(GobFilePath)
	fmt.Println("Shutdown complete")
}

func sleep(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done(): // context cancelled
		return true
	case <-time.After(duration):
		return false
	}
}

// Map Saver service

type MapSaver struct {
	cancel context.CancelFunc
}

func (m *MapSaver) Stop() {
	m.cancel()
}

func NewMapSaver(wg *sync.WaitGroup) Service {
	m := &MapSaver{}
	ctx, cancelFunc := context.WithCancel(context.Background())
	m.cancel = cancelFunc
	m0, q0 := Map.Stats()
	wg.Add(1)
	go func() {
		for {
			// save the map every 5 minutes, if modified
			if sleep(ctx, 5*time.Minute) {
				wg.Done()
				return
			}
			m1, q1 := Map.Stats()
			if m1 != m0 || q1 != q0 { // has changed?
				m.saveNetworkMap()
				m0 = m1
				q0 = q1
			}
		}
	}()
	return m
}

func (m *MapSaver) saveNetworkMap() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Error saving network map: %v", err)
		}
	}()
	Map.WriteGob(GobFilePath)
}

// Address Collector service

type Collector struct {
	cancel  context.CancelFunc
	conn    net.Conn
	address string
}

func (c *Collector) Stop() {
	c.cancel()
	if c.conn != nil {
		// must close net.Conn to interrupt blocking read/write.
		c.conn.Close()
	}
}

func NewCollector(wg *sync.WaitGroup, fromAddr string, maxTime time.Duration) Service {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Collector{cancel: cancel, address: fromAddr}
	wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[%v] error: %v", c.address, err)
			}
		}()
		for {
			// choose the next node to connect to
			remoteNode := fromAddr
			if remoteNode == "" {
				remoteNode = Map.ChooseNode()
			}
			c.address = remoteNode // for recover
			// collect addresses from the node until the timeout
			c.collectAddresses(ctx, remoteNode, maxTime)
			// avoid spamming on connect errors
			if sleep(ctx, 10*time.Second) {
				// context was cancelled
				wg.Done()
				return
			}
		}
	}()
	return c
}

func (c *Collector) collectAddresses(ctx context.Context, nodeAddr string, maxTime time.Duration) {
	who := nodeAddr
	fmt.Printf("[%s] Connecting to node: %s\n", who, nodeAddr)

	d := net.Dialer{Timeout: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", nodeAddr)
	if err != nil {
		fmt.Printf("[%s] Error connecting to Dogecoin node: %v\n", who, err)
		return
	}
	defer conn.Close()
	c.conn = conn // for shutdown

	// set a time limit on waiting for addresses per node
	if maxTime != 0 {
		conn.SetReadDeadline(time.Now().Add(maxTime))
	}
	reader := bufio.NewReader(conn)

	// send our 'version' message
	_, err = conn.Write(msg.EncodeMessage("version", PawVersionMessage(CurrentProtocolVersion))) // nodeVer
	if err != nil {
		fmt.Printf("[%s] Error sending version message: %v\n", who, err)
		return
	}

	fmt.Printf("[%s] Sent 'version' message\n", who)

	// expect the version message from the node
	version, err := expectVersion(reader)
	if err != nil {
		fmt.Printf("[%s] %v", who, err)
		return
	}

	fmt.Printf("[%s] Received 'version': %v\n", who, version)

	nodeVer := version.Version // other node's version
	if nodeVer >= 209 {
		// send 'verack' in response
		_, err = conn.Write(msg.EncodeMessage("verack", []byte{}))
		if err != nil {
			fmt.Printf("[%s] failed to send 'verack': %v\n", who, err)
			return
		}
		fmt.Printf("[%s] Sent 'verack'\n", who)
	}

	addresses := 0
	for {
		cmd, payload, err := msg.ReadMessage(reader)
		if err != nil {
			fmt.Printf("[%s] Error reading message: %v\n", who, err)
			return
		}

		switch cmd {
		case "ping":
			fmt.Printf("[%s] Ping received.\n", who)
			sendPong(conn, payload, who) // keep-alive

			// request a big list of known addresses (addr response)
			sendGetAddr(conn, who)
			fmt.Printf("[%s] Sent getaddr.\n", who)

		case "reject":
			re := msg.DecodeReject(payload)
			fmt.Printf("[%s] Reject: %v %v %v\n", who, re.CodeName(), re.Message, re.Reason)

		case "addr":
			addr := msg.DecodeAddrMsg(payload, nodeVer)
			_, oldLen := Map.Stats()
			kept := 0
			keepAfter := time.Now().Add(-ExpiryTime).Unix()
			for _, a := range addr.AddrList {
				// fmt.Println("• ", net.IP(a.Address), a.Port, "svc", a.Services, "ts", a.Time)
				if int64(a.Time) >= keepAfter {
					Map.AddNode(net.IP(a.Address), a.Port, a.Time, a.Services)
					kept++
				}
			}
			mapSize, newLen := Map.Stats()
			fmt.Printf("[%s] Addresses: %d received, %d expired, %d new, %d in map\n", who, len(addr.AddrList), len(addr.AddrList)-kept, (newLen - oldLen), mapSize)
			addresses += len(addr.AddrList)
			if addresses >= 1000 && maxTime != 0 {
				// done: try the next node (not if local node, i.e. maxTime=0)
				conn.Close()
				return
			}

		default:
			//fmt.Printf("Command '%s' payload: %s\n", cmd, hex.EncodeToString(payload))
			fmt.Printf("[%s] Received: %v\n", who, cmd)
		}
	}
}

func expectVersion(reader *bufio.Reader) (msg.VersionMsg, error) {
	// Core Node implementation: if connection is inbound, send Version immediately.
	// This means we'll receive the Node's version before `verack` for our Version,
	// however this is undocumented, so other nodes might ack first.
	cmd, payload, err := msg.ReadMessage(reader)
	if err != nil {
		return msg.VersionMsg{}, fmt.Errorf("error reading message: %v", err)
	}
	if cmd == "version" {
		return msg.DecodeVersion(payload), nil
	}
	if cmd == "reject" {
		re := msg.DecodeReject(payload)
		return msg.VersionMsg{}, fmt.Errorf("reject: %s %s %s", re.CodeName(), re.Message, re.Reason)
	}
	return msg.VersionMsg{}, fmt.Errorf("expected 'version' message from node, but received: %s", cmd)
}

func sendPong(conn net.Conn, pingPayload []byte, who string) {
	// reply with 'pong', same payload (nonce)
	_, err := conn.Write(msg.EncodeMessage("pong", pingPayload))
	if err != nil {
		fmt.Printf("[%s] failed to send 'pong': %v", who, err)
		return
	}
}

func sendGetAddr(conn net.Conn, who string) {
	_, err := conn.Write(msg.EncodeMessage("getaddr", []byte{}))
	if err != nil {
		fmt.Printf("[%s] failed to send 'getaddr': %v", who, err)
		return
	}
}
