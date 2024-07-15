package gossip

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/dogeorg/dogenet/internal/gossip/protocol"
	"github.com/dogeorg/dogenet/internal/governor"
	"github.com/dogeorg/dogenet/internal/spec"
)

type netService struct {
	governor.ServiceCtx
	address     spec.Address
	mutex       sync.Mutex
	listner     net.Listener
	channels    map[protocol.Tag4CC]chan protocol.Message
	privKey     protocol.PrivKey
	identPub    protocol.PubKey
	remotePort  uint16
	connections map[net.Conn]struct{}
}

func New(address spec.Address, remotePort uint16) governor.Service {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("cannot generate pubkey: %v", err))
	}
	if remotePort == 0 {
		remotePort = spec.DogeNetDefaultPort
	}
	return &netService{
		address:     address,
		connections: make(map[net.Conn]struct{}),
		privKey:     priv,
		identPub:    pub,
		remotePort:  remotePort,
	}
}

func (ns *netService) Run() {
	var lc net.ListenConfig
	who := ns.address.String()
	listner, err := lc.Listen(ns.Context, "tcp", ns.address.String())
	if err != nil {
		log.Printf("[%s] cannot listen on `%v`: %v", ns.ServiceName, who, err)
		return
	}
	log.Printf("[%s] listening on %v", ns.ServiceName, who)
	ns.mutex.Lock()
	ns.listner = listner
	ns.mutex.Unlock()
	defer listner.Close()
	for {
		conn, err := listner.Accept()
		if err != nil {
			log.Printf("[%s] accept failed on `%v`: %v", ns.ServiceName, who, err)
			return // typically due to Stop()
		}
		if ns.trackConn(conn, true) {
			go func(conn net.Conn) {
				defer func(conn net.Conn) { conn.Close(); ns.trackConn(conn, false) }(conn)
				ns.inboundConnection(conn)
			}(conn)
		} else { // Stop was called
			conn.Close()
			return
		}
	}
}

func (ns *netService) Stop() {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	// stop accepting connections
	if ns.listner != nil {
		ns.listner.Close()
	}
	// close all active connections
	for c := range ns.connections {
		c.Close()
	}
}

func (ns *netService) trackConn(c net.Conn, add bool) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if add {
		if ns.Stopping() {
			return false
		}
		ns.connections[c] = struct{}{}
	} else {
		delete(ns.connections, c)
	}
	return true
}

func (ns *netService) inboundConnection(conn net.Conn) {
	// receive messages from peer
	// who := ns.address.String()
	reader := bufio.NewReader(conn)
	ns.sendMyAddress(conn)
	for {
		msg, err := protocol.ReadMessage(reader)
		if err != nil {
			log.Printf("bad message: %v", err)
			return // close connection
		}
		// dispatch the message to the channel listener
		if channel, ok := ns.channels[msg.Chan]; ok {
			channel <- msg
		}

		// switch tag {
		// case protocol.TagReject:
		// 	rej := protocol.DecodeReject(msg)
		// 	if rej.Code == protocol.REJECT_TAG {
		// 		fmt.Printf("[%s] Reject tag: %s %s\n", who, string(rej.Data), rej.Reason)
		// 	} else {
		// 		fmt.Printf("[%s] Reject: %s %s %s\n", who, rej.CodeName(), rej.Reason, hex.EncodeToString(rej.Data))
		// 	}

		// default:
		// 	_, err := conn.Write(
		// 		protocol.EncodeMessage(protocol.TagReject, ns.privKey,
		// 			protocol.EncodeReject(protocol.REJECT_TAG, "not supported", protocol.TagToBytes(tag))))
		// 	if err != nil {
		// 		log.Printf("failed to send message")
		// 		return // drop connection
		// 	}
		// }
	}
}

func (ns *netService) sendMyAddress(conn net.Conn) {
	addr := protocol.AddressMsg{
		Time:    protocol.DogeNow(),
		Address: ns.address.Host.To16(), // can return nil
		Port:    ns.address.Port,
		Channels: []protocol.Tag4CC{
			protocol.ChannelIdentity,
			protocol.ChannelB0rk,
		},
		Services: []protocol.Service{
			{Tag: protocol.ServiceCore, Port: 22556, Data: ""},
		},
	}
	addr.Owner = ns.identPub // shared!
	conn.Write(protocol.EncodeMessage(protocol.ChannelDoge, protocol.TagAddress, ns.privKey, addr.Encode()))
}
