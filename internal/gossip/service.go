package gossip

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/dogeorg/dogenet/internal/gossip/protocol"
	"github.com/dogeorg/dogenet/internal/governor"
	"github.com/dogeorg/dogenet/internal/spec"
)

const IdealPeers = 8

type netService struct {
	governor.ServiceCtx
	address     spec.Address
	mutex       sync.Mutex
	listner     net.Listener
	channels    map[protocol.Tag4CC]chan protocol.Message
	store       spec.Store
	privKey     protocol.PrivKey
	identPub    protocol.PubKey
	remotePort  uint16
	connections map[net.Conn]spec.Address
}

func New(address spec.Address, remotePort uint16, store spec.Store) governor.Service {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("cannot generate pubkey: %v", err))
	}
	if remotePort == 0 {
		remotePort = spec.DogeNetDefaultPort
	}
	return &netService{
		address:     address,
		channels:    make(map[protocol.Tag4CC]chan protocol.Message),
		store:       store,
		privKey:     priv,
		identPub:    pub,
		remotePort:  remotePort,
		connections: make(map[net.Conn]spec.Address),
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
	go ns.attractPeers()
	defer listner.Close()
	for {
		conn, err := listner.Accept()
		if err != nil {
			log.Printf("[%s] accept failed on `%v`: %v", ns.ServiceName, who, err)
			return // typically due to Stop()
		}
		remote, err := spec.ParseAddress(conn.RemoteAddr().String())
		if err != nil {
			log.Printf("[%s] no remote address for inbound peer: %v", who, err)
		}
		if ns.trackConn(conn, remote) {
			newPeer(conn, remote, ns, ns.privKey, ns.identPub)
		} else { // Stop was called
			conn.Close()
			return
		}
	}
}

// goroutine
func (ns *netService) attractPeers() {
	who := "attract-peers"
	for !ns.Stopping() {
		if ns.countPeers() < IdealPeers {
			addr := ns.store.ChooseNetNode()
			log.Printf("[%s] choosing peer: %v", who, addr)
			if addr.IsValid() && !ns.havePeer(addr) {
				// attempt to connect to the peer
				d := net.Dialer{Timeout: 30 * time.Second}
				conn, err := d.DialContext(ns.Context, "tcp", addr.String())
				if err != nil {
					log.Printf("[%s] connect failed: %v", who, err)
				} else {
					if ns.trackConn(conn, addr) {
						newPeer(conn, addr, ns, ns.privKey, ns.identPub)
						continue
					} else { // Stop was called
						conn.Close()
						return
					}
				}
			}
		}
		// no peer required, or no peer available.
		ns.Sleep(60 * time.Second)
	}
}

func (ns *netService) countPeers() int {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	return len(ns.connections)
}

func (ns *netService) havePeer(remote spec.Address) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for _, addr := range ns.connections {
		if addr.Host.Equal(remote.Host) && addr.Port == remote.Port {
			return true
		}
	}
	return false
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

func (ns *netService) closeConn(conn net.Conn) {
	conn.Close()
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	delete(ns.connections, conn)
}

func (ns *netService) trackConn(conn net.Conn, addr spec.Address) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	ns.connections[conn] = addr
	return true
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
