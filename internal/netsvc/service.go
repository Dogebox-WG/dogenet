package netsvc

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"rad/gossip/dnet"
	"rad/governor"

	"github.com/dogeorg/dogenet/internal/spec"
)

const IdealPeers = 8
const ProtocolSocket = "/tmp/dogenet.sock"

type netService struct {
	governor.ServiceCtx
	address     spec.Address
	mutex       sync.Mutex
	listner     net.Listener
	socket      net.Listener
	channels    map[dnet.Tag4CC]chan dnet.Message
	store       spec.Store
	privKey     dnet.PrivKey
	identPub    dnet.PubKey
	remotePort  uint16
	connections map[net.Conn]spec.Address
	peers       []*peerConn
	handlers    []*handlerConn
}

func New(address spec.Address, remotePort uint16, store spec.Store) governor.Service {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("cannot generate pubkey: %v", err))
	}
	log.Printf("Node PubKey is: %v", hex.EncodeToString(pub))
	if remotePort == 0 {
		remotePort = dnet.DogeNetDefaultPort
	}
	return &netService{
		address:     address,
		channels:    make(map[dnet.Tag4CC]chan dnet.Message),
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
	go ns.acceptHandlers()
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
		peer := newPeer(conn, remote, ns, ns.privKey, ns.identPub)
		if ns.trackPeer(peer) {
			log.Printf("[%s] peer connected: %v", who, remote)
			peer.start()
		} else { // Stop was called
			conn.Close()
			return
		}
	}
}

// goroutine
func (ns *netService) acceptHandlers() {
	who := "accept-handlers"
	var err error
	os.Remove(ProtocolSocket)
	ns.socket, err = net.Listen("unix", ProtocolSocket)
	if err != nil {
		log.Printf("[%s] cannot create unix socket %s: %v", who, ProtocolSocket, err)
		return
	}
	for !ns.Stopping() {
		// Accept an incoming connection.
		conn, err := ns.socket.Accept()
		if err != nil {
			log.Fatal(err)
		}
		hand := newHandler(conn, ns)
		if ns.trackHandler(hand) {
			log.Printf("[%s] handler connected.", who)
			hand.start()
		} else {
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
					peer := newPeer(conn, addr, ns, ns.privKey, ns.identPub)
					if ns.trackPeer(peer) {
						peer.start()
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

// called from attractPeers
func (ns *netService) countPeers() int {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	return len(ns.peers)
}

// called from attractPeers
func (ns *netService) havePeer(remote spec.Address) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for _, peer := range ns.peers {
		if peer.address.Host.Equal(remote.Host) && peer.address.Port == remote.Port {
			return true
		}
	}
	return false
}

// called from any
func (ns *netService) Stop() {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	// stop accepting network connections
	if ns.listner != nil {
		ns.listner.Close()
	}
	// stop accepting handler connections
	if ns.socket != nil {
		ns.socket.Close()
		os.Remove(ProtocolSocket)
	}
	// close all active connections
	for c := range ns.connections {
		c.Close()
	}
}

// called from any
func (ns *netService) forwardToPeers(msg dnet.Message) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for _, peer := range ns.peers {
		// non-blocking send to peer
		select {
		case peer.send <- msg:
		default:
		}
	}
}

// called from any
func (ns *netService) forwardToHandlers(msg dnet.Message) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	found := false
	for _, hand := range ns.handlers {
		// check if the handler is listening on this channel
		if uint32(msg.Chan) == atomic.LoadUint32(&hand.channel) {
			// non-blocking send to handler
			select {
			case hand.send <- msg:
				// after accepting this message into the queue,
				// the handler becomes responsible for sending a reject
				// (however there can be multiple handlers!)
				found = true
			default:
			}
		}
	}
	return found
}

// called from any
func (ns *netService) closePeer(peer *peerConn) {
	peer.conn.Close()
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for i, p := range ns.peers {
		if p == peer {
			// remove from unordered array
			ns.peers[i] = ns.peers[len(ns.peers)-1]
			ns.peers = ns.peers[:len(ns.peers)-1]
			break
		}
	}
}

// called from any
func (ns *netService) closeHandler(hand *handlerConn) {
	hand.conn.Close()
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for i, h := range ns.handlers {
		if h == hand {
			// remove from unordered array
			ns.handlers[i] = ns.handlers[len(ns.handlers)-1]
			ns.handlers = ns.handlers[:len(ns.handlers)-1]
			break
		}
	}
}

// called from any
func (ns *netService) trackPeer(peer *peerConn) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	ns.peers = append(ns.peers, peer)
	return true
}

// called from any
func (ns *netService) trackHandler(hand *handlerConn) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	ns.handlers = append(ns.handlers, hand)
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
