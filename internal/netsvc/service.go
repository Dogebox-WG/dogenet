package netsvc

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/governor"

	"code.dogecoin.org/dogenet/internal/spec"
)

const IdealPeers = 8
const ProtocolSocket = "/tmp/dogenet.sock"

type NetService struct {
	governor.ServiceCtx
	bindAddrs   []spec.Address // bind-to address on THIS node
	pubAddr     spec.Address   // public address of THIS node
	mutex       sync.Mutex
	listen      []net.Listener
	socket      net.Listener
	channels    map[dnet.Tag4CC]chan dnet.Message
	store       spec.Store
	nodeKey     dnet.KeyPair
	idenPub     dnet.PubKey
	connections map[net.Conn]spec.Address
	peers       []*peerConn
	handlers    []*handlerConn
	newPeers    chan spec.NodeInfo
}

func New(bind []spec.Address, pubAddr spec.Address, store spec.Store) spec.NetSvc {
	nodeKey, err := dnet.GenerateKeyPair()
	if err != nil {
		panic(fmt.Sprintf("cannot generate node keypair: %v", err))
	}
	idenKey, err := dnet.GenerateKeyPair()
	if err != nil {
		panic(fmt.Sprintf("cannot generate iden keypair: %v", err))
	}
	log.Printf("Node PubKey is: %v", hex.EncodeToString(nodeKey.Pub))
	log.Printf("Iden PubKey is: %v", hex.EncodeToString(idenKey.Pub))
	return &NetService{
		bindAddrs:   bind,
		pubAddr:     pubAddr,
		channels:    make(map[dnet.Tag4CC]chan dnet.Message),
		store:       store,
		nodeKey:     nodeKey,
		idenPub:     idenKey.Pub,
		connections: make(map[net.Conn]spec.Address),
		newPeers:    make(chan spec.NodeInfo, 10),
	}
}

// goroutine
func (ns *NetService) Run() {
	var wg sync.WaitGroup
	ns.startListeners(&wg)
	go ns.acceptHandlers()
	go ns.attractPeers()
	wg.Wait()
}

// Attempt to add a known peer from the command-line or REST API.
// This attempts to connect to the peer (in a goroutine) and adds
// the peer to the database if connection is successful.
func (ns *NetService) AddPeer(pubKey spec.PubKey, addr spec.Address) {
	ns.newPeers <- spec.NodeInfo{PubKey: pubKey, Addr: addr}
}

// on 'Run' goroutine
func (ns *NetService) startListeners(wg *sync.WaitGroup) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for _, b := range ns.bindAddrs {
		lc := net.ListenConfig{
			KeepAlive: -1, // use protocol-level pings
		}
		listner, err := lc.Listen(ns.Context, "tcp", b.String())
		if err != nil {
			log.Printf("[%s] cannot listen on `%v`: %v", ns.ServiceName, b.String(), err)
			continue
		}
		log.Printf("[%s] listening on %v", ns.ServiceName, b.String())
		if ns.Stopping() {
			listner.Close()
			return // shutting down
		}
		ns.listen = append(ns.listen, listner)
		wg.Add(1)
		go ns.acceptIncoming(listner, b.String(), wg)
	}
}

// goroutine
func (ns *NetService) acceptIncoming(listner net.Listener, who string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer listner.Close()
	for {
		conn, err := listner.Accept()
		if err != nil {
			log.Printf("[%s] accept failed on `%v`: %v", ns.ServiceName, who, err)
			return // typically due to Stop()
		}
		remote, err := dnet.ParseAddress(conn.RemoteAddr().String())
		if err != nil {
			log.Printf("[%s] no remote address for inbound peer: %v", who, err)
		}
		peer := newPeer(conn, remote, nil, ns) // inbound connection (no peerPub)
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
func (ns *NetService) acceptHandlers() {
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
func (ns *NetService) attractPeers() {
	who := "attract-peers"
	for !ns.Stopping() {
		node := ns.choosePeer()
		log.Printf("[%s] choosing peer: %v [%v]", who, node.Addr, hex.EncodeToString(node.PubKey))
		if node.IsValid() {
			// attempt to connect to the peer
			d := net.Dialer{Timeout: 30 * time.Second}
			conn, err := d.DialContext(ns.Context, "tcp", node.Addr.String())
			if err != nil {
				log.Printf("[%s] connect failed: %v", who, err)
			} else {
				peer := newPeer(conn, node.Addr, node.PubKey, ns) // outbound connection (have PeerPub)
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
}

// called from attractPeers
func (ns *NetService) choosePeer() spec.NodeInfo {
	for !ns.Stopping() {
		select {
		case np := <-ns.newPeers: // from ns.AddPeer()
			return np
		default:
			if ns.countPeers() < IdealPeers {
				np := ns.store.ChooseNetNode()
				if np.IsValid() {
					if ns.havePeer(np.PubKey) {
						continue // pick again
					}
					return np
				}
			}
		}
		// no peer available/required: sleep while receiving.
		select {
		case np := <-ns.newPeers: // from ns.AddPeer()
			return np
		case <-time.After(60 * time.Second):
			continue
		}
	}
	return spec.NodeInfo{}
}

// called from attractPeers
func (ns *NetService) countPeers() int {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	return len(ns.peers)
}

// called from attractPeers
func (ns *NetService) havePeer(remote dnet.PubKey) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for _, peer := range ns.peers {
		if peer.peerPub != nil && bytes.Equal(peer.peerPub, remote) {
			return true
		}
	}
	return false
}

// called from any
func (ns *NetService) Stop() {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	// stop accepting network connections
	for _, listner := range ns.listen {
		listner.Close()
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
func (ns *NetService) forwardToPeers(rawMsg []byte) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	for _, peer := range ns.peers {
		// non-blocking send to peer
		select {
		case peer.send <- rawMsg:
		default:
		}
	}
}

// called from any
func (ns *NetService) forwardToHandlers(channel dnet.Tag4CC, rawMsg []byte) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	found := false
	for _, hand := range ns.handlers {
		// check if the handler is listening on this channel
		if uint32(channel) == atomic.LoadUint32(&hand.channel) {
			// non-blocking send to handler
			select {
			case hand.send <- rawMsg:
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
func (ns *NetService) closePeer(peer *peerConn) {
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
func (ns *NetService) closeHandler(hand *handlerConn) {
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
func (ns *NetService) trackPeer(peer *peerConn) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	ns.peers = append(ns.peers, peer)
	return true
}

// called from any
func (ns *NetService) trackHandler(hand *handlerConn) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	ns.handlers = append(ns.handlers, hand)
	return true
}
