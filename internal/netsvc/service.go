package netsvc

import (
	"encoding/hex"
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
const PeerLockTime = 300 * time.Second // 5 minutes

type NetService struct {
	governor.ServiceCtx
	bindAddrs  []spec.Address // bind-to address on THIS node
	publicAddr spec.Address   // public address of THIS node
	allowLocal bool           // allow local IP address in Announcement messages (for local testing)
	store      spec.Store
	cstore     spec.StoreCtx
	nodeKey    dnet.KeyPair
	newPeers   chan spec.NodeInfo
	// MUTEX state:
	mutex          sync.Mutex
	connections    []net.Conn              // all current network connections (peers and handlers)
	listen         []net.Listener          // listen sockets for peers to connect
	connectedPeers map[MapPubKey]*peerConn // currently connected peers by pubkey
	lockedPeers    map[MapPubKey]time.Time // peer pubkeys locked for a short time during connection attempts
	socket         net.Listener            // listen socket for handlers to connect
	handlers       []*handlerConn          // currently connected handlers
	encAnnounce    spec.RawMessage         // current encoded announcement, ready for sending to peers (mutex)
}

type MapPubKey = [32]byte

var NoPubKey [32]byte // zeroes

func New(bind []spec.Address, public spec.Address, idenPub dnet.PubKey, store spec.Store, nodeKey dnet.KeyPair, allowLocal bool) spec.NetSvc {
	return &NetService{
		bindAddrs:      bind,
		allowLocal:     allowLocal,
		store:          store,
		nodeKey:        nodeKey,
		lockedPeers:    make(map[MapPubKey]time.Time),
		connectedPeers: make(map[MapPubKey]*peerConn),
		newPeers:       make(chan spec.NodeInfo, 10),
	}
}

// goroutine
func (ns *NetService) Run() {
	ns.cstore = ns.store.WithCtx(ns.Context) // Service Context is first available here
	var wg sync.WaitGroup
	ns.startListeners(&wg)
	go ns.acceptHandlers()
	go ns.findPeers()
	wg.Wait()
}

// Attempt to add a known peer from the command-line or REST API.
// This attempts to connect to the peer (in a goroutine) and adds
// the peer to the database if connection is successful.
func (ns *NetService) AddPeer(node spec.NodeInfo) {
	ns.newPeers <- node
}

// ReceiveAnnounce implements AnnounceReceiver.
// Receives signed announcement messages from the Announce service.
func (ns *NetService) ReceiveAnnounce(msg spec.RawMessage) {
	ns.setAnnounce(msg)
	ns.forwardToPeers(msg)
}

// called from any peer
func (ns *NetService) GetAnnounce() spec.RawMessage {
	ns.mutex.Lock() // vs setAnnounce
	defer ns.mutex.Unlock()
	return ns.encAnnounce
}

func (ns *NetService) setAnnounce(msg spec.RawMessage) {
	ns.mutex.Lock() // vs getAnnounce
	defer ns.mutex.Unlock()
	ns.encAnnounce = msg
}

// on 'Run' goroutine
func (ns *NetService) startListeners(wg *sync.WaitGroup) {
	ns.mutex.Lock() // vs Stop
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
		peer := newPeer(conn, remote, NoPubKey, false, ns) // inbound connection
		if ns.trackPeer(conn, peer, NoPubKey) {
			log.Printf("[%s] peer connected (inbound): %v", who, remote)
			// this peer will call adoptPeer once is receives the peer pubKey.
			peer.start()
		} else { // Stop was called
			log.Printf("[%s] dropped peer, shutting down: %v", who, remote)
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
		if ns.trackHandler(conn, hand) {
			log.Printf("[%s] handler connected.", who)
			hand.start()
		} else {
			conn.Close()
			return
		}
	}
}

// goroutine
func (ns *NetService) findPeers() {
	who := "find-peers"
	for !ns.Stopping() {
		node := ns.choosePeer(who) // blocking
		pubHex := hex.EncodeToString(node.PubKey[:])
		if node.IsValid() && !ns.havePeer(node.PubKey) && ns.lockPeer(node.PubKey) {
			log.Printf("[%s] choosing peer: %v [%v]", who, node.Addr, pubHex)
			// attempt to connect to the peer
			d := net.Dialer{Timeout: 30 * time.Second}
			conn, err := d.DialContext(ns.Context, "tcp", node.Addr.String())
			if err != nil {
				log.Printf("[%s] connect failed: %v", who, err)
			} else {
				peer := newPeer(conn, node.Addr, node.PubKey, true, ns) // outbound connection
				if ns.trackPeer(conn, peer, node.PubKey) {
					log.Printf("[%s] connected to peer (outbound): %v [%v]", who, node.Addr, pubHex)
					peer.start()
				} else { // already connected to peer, or Stop was called
					log.Printf("[%s] dropped peer, already connected (outbound): %v [%v]", who, node.Addr, pubHex)
					conn.Close()
					return
				}
			}
		}
	}
}

// called from attractPeers
func (ns *NetService) choosePeer(who string) spec.NodeInfo {
	for !ns.Stopping() {
		select {
		case np := <-ns.newPeers: // from ns.AddPeer()
			return np
		default:
			if ns.countPeers() < IdealPeers {
				ns.Sleep(time.Second) // avoid spinning
				np, err := ns.cstore.ChooseNetNode()
				if err != nil {
					log.Printf("[%s] ChooseNetNode: %v", who, err)
				} else {
					return np
				}
			}
		}
		// no peer available/required: sleep while receiving.
		select {
		case np := <-ns.newPeers: // from ns.AddPeer()
			return np
		case <-time.After(30 * time.Second):
			continue
		}
	}
	return spec.NodeInfo{}
}

// called from any
func (ns *NetService) Stop() {
	ns.mutex.Lock() // vs startListeners, acceptHandlers, any track/close
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
	for _, c := range ns.connections {
		c.Close()
	}
}

// called from any
func (ns *NetService) forwardToPeers(msg spec.RawMessage) {
	ns.mutex.Lock() // vs countPeers,havePeer,trackPeer,adoptPeer,closePeer
	defer ns.mutex.Unlock()
	for _, peer := range ns.connectedPeers {
		// non-blocking send to peer
		select {
		case peer.send <- msg:
		default:
		}
	}
}

// called from any
func (ns *NetService) forwardToHandlers(channel dnet.Tag4CC, rawHdr []byte, payload []byte) bool {
	ns.mutex.Lock() // vs trackHandler,closeHandler
	defer ns.mutex.Unlock()
	found := false
	for _, hand := range ns.handlers {
		// check if the handler is listening on this channel
		if uint32(channel) == atomic.LoadUint32(&hand.channel) {
			// non-blocking send to handler
			select {
			case hand.send <- spec.RawMessage{Header: rawHdr, Payload: payload}:
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

// called from attractPeers
func (ns *NetService) countPeers() int {
	ns.mutex.Lock() // vs havePeer,trackPeer,adoptPeer,closePeer,forwardToPeers
	defer ns.mutex.Unlock()
	return len(ns.connectedPeers)
}

// lockPeer reserves a peer PubKey for PeerLockTime (for connection attempts)
// this prevents connecting to the same peer over and over
// called from attractPeers
func (ns *NetService) lockPeer(pubKey MapPubKey) bool {
	ns.mutex.Lock() // vs ?? (lockedPeers is private to findPeers)
	defer ns.mutex.Unlock()
	now := time.Now()
	if until, have := ns.lockedPeers[pubKey]; have {
		if now.Before(until) {
			return false // still locked
		}
	}
	// lock the peer
	ns.lockedPeers[pubKey] = now.Add(PeerLockTime)
	return true
}

// havePeer returns true if we're already connected to a peer with pubKey
// called from attractPeers
func (ns *NetService) havePeer(pubKey MapPubKey) bool {
	ns.mutex.Lock() // vs countPeers,trackPeer,adoptPeer,closePeer,forwardToPeers
	defer ns.mutex.Unlock()
	_, have := ns.connectedPeers[pubKey]
	return have
}

// trackPeer adds a peer to our set of connected peers
// called from any
// returns false if service is stopping
func (ns *NetService) trackPeer(conn net.Conn, peer *peerConn, pubKey MapPubKey) bool {
	ns.mutex.Lock() // vs countPeers,havePeer,adoptPeer,closePeer,forwardToPeers,Stop
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	// begin tracking the connection
	ns.connections = append(ns.connections, conn)
	// check if connected before tracking the peer
	if pubKey != NoPubKey {
		if _, have := ns.connectedPeers[pubKey]; have {
			return false // already connected to peer
		}
		// mark peer connected: affects future havePeer(), adoptPeer(), trackPeer() results
		ns.connectedPeers[pubKey] = peer
	}
	return true
}

// adoptPeer sets peer's PubKey if we're not already connected to that peer
// called from any peer.receiveFromPeer
func (ns *NetService) adoptPeer(peer *peerConn, pubKey MapPubKey) bool {
	ns.mutex.Lock() // vs countPeers,havePeer,trackPeer,closePeer,forwardToPeers
	defer ns.mutex.Unlock()
	if _, have := ns.connectedPeers[pubKey]; have {
		return false // already connected to peer
	}
	// mark peer connected: affects future havePeer(), adoptPeer(), trackPeer() results
	ns.connectedPeers[pubKey] = peer
	return true
}

// called from any peer
func (ns *NetService) closePeer(peer *peerConn) {
	conn := peer.conn
	conn.Close()
	ns.mutex.Lock() // vs countPeers,havePeer,trackPeer,adoptPeer,forwardToPeers,Stop
	defer ns.mutex.Unlock()
	// remove the peer connected status
	log.Printf("[%v] closing connection to peer: %v", peer.addr.String(), hex.EncodeToString(peer.peerPub[:]))
	key := peer.peerPub
	if p, have := ns.connectedPeers[key]; have && p == peer {
		delete(ns.connectedPeers, key)
	}
	// remove the tracked connnection
	for i, c := range ns.connections {
		if c == conn {
			// remove from unordered array
			ns.connections[i] = ns.connections[len(ns.connections)-1]
			ns.connections = ns.connections[:len(ns.connections)-1]
			break
		}
	}
}

// trackHandler adds a handler connection to our tracking array
// called from any
// returns false if service is stopping
func (ns *NetService) trackHandler(conn net.Conn, hand *handlerConn) bool {
	ns.mutex.Lock() // vs closeHandler,forwardToHandlers,Stop
	defer ns.mutex.Unlock()
	if ns.Stopping() {
		return false
	}
	// begin tracking the connection
	ns.connections = append(ns.connections, conn)
	// begin tracking the handler instance
	ns.handlers = append(ns.handlers, hand)
	return true
}

// called from any
func (ns *NetService) closeHandler(hand *handlerConn) {
	conn := hand.conn
	conn.Close()
	ns.mutex.Lock() // vs trackHandler,forwardToHandlers,Stop
	defer ns.mutex.Unlock()
	// remove the tracked connnection
	for i, c := range ns.connections {
		if c == conn {
			// remove from unordered array
			ns.connections[i] = ns.connections[len(ns.connections)-1]
			ns.connections = ns.connections[:len(ns.connections)-1]
			break
		}
	}
	// remove the handler instance
	for i, h := range ns.handlers {
		if h == hand {
			// remove from unordered array
			ns.handlers[i] = ns.handlers[len(ns.handlers)-1]
			ns.handlers = ns.handlers[:len(ns.handlers)-1]
			break
		}
	}
}
