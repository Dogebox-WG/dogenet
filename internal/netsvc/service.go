package netsvc

import (
	"encoding/hex"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/gossip/node"
	"code.dogecoin.org/governor"

	"code.dogecoin.org/dogenet/internal/spec"
)

const IdealPeers = 8
const PeerLockTime = 30 * time.Second          // was 5 minutes, now 30 seconds
const SeedAttemptTime = 60 * time.Second       // time between seed connect attempts
const SeedAttemptRandom = 10                   // randomness in the interval, in seconds
const GossipAddressInverval = 60 * time.Second // gossip a random address to the peer
const GossipAddressRandom = 10                 // randomness in the interval, in seconds
const SeedDNS = "seed.dogecoin.org"            // seed peer addresses (DNS lookup)

type NetService struct {
	governor.ServiceCtx
	bindAddrs       []spec.Address // bind-to address on THIS node
	handlerBind     spec.BindTo
	allowLocal      bool // allow local IP address in Announcement messages (for local testing)
	_store          spec.Store
	store           spec.Store
	nodeKey         dnet.KeyPair
	newPeers        chan spec.NodeInfo
	announceChanges chan any // send spec.Change* to Announce service
	// MUTEX state:
	mutex          sync.Mutex
	connections    []net.Conn              // all current network connections (peers and handlers)
	listen         []net.Listener          // listen sockets for peers to connect
	connectedPeers map[MapPubKey]*peerConn // currently connected peers by pubkey
	lockedPeers    map[MapPubKey]time.Time // peer pubkeys locked for a short time during connection attempts
	socket         net.Listener            // listen socket for handlers to connect
	handlers       []*handlerConn          // currently connected handlers
	encAnnounce    dnet.RawMessage         // current encoded announcement, ready for sending to peers (mutex)
}

type MapPubKey = [32]byte

var NoPubKey [32]byte // zeroes

func New(bind []spec.Address, handlerBind spec.BindTo, nodeKey dnet.KeyPair, store spec.Store, allowLocal bool, announceChanges chan any) spec.NetSvc {
	return &NetService{
		bindAddrs:       bind,
		handlerBind:     handlerBind,
		allowLocal:      allowLocal,
		_store:          store,
		nodeKey:         nodeKey,
		lockedPeers:     make(map[MapPubKey]time.Time),
		connectedPeers:  make(map[MapPubKey]*peerConn),
		newPeers:        make(chan spec.NodeInfo, 10),
		announceChanges: announceChanges, // used in handler
	}
}

// goroutine
func (ns *NetService) Run() {
	ns.store = ns._store.WithCtx(ns.Context) // Service Context is first available here
	var wg sync.WaitGroup
	ns.startListeners(&wg)
	go ns.acceptHandlers()
	go ns.findPeers()
	go ns.gossipRandomAddresses()
	go ns.seedFromDNS()
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
func (ns *NetService) ReceiveAnnounce(msg dnet.RawMessage) {
	ns.setAnnounce(msg)
	ns.forwardToPeers(msg)
}

// called from any peer
func (ns *NetService) GetAnnounce() dnet.RawMessage {
	ns.mutex.Lock() // vs setAnnounce
	defer ns.mutex.Unlock()
	return ns.encAnnounce
}

func (ns *NetService) setAnnounce(msg dnet.RawMessage) {
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
		peer := newPeer(conn, remote, NoPubKey, false, false, ns) // inbound connection
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
	if ns.handlerBind.Network == "unix" {
		// in case the prior process was killed.
		os.Remove(ns.handlerBind.Address)
	}
	ns.socket, err = net.Listen(ns.handlerBind.Network, ns.handlerBind.Address)
	if err != nil {
		log.Printf("[%s] cannot listen on %v: %v", who, ns.handlerBind, err)
		return
	}
	log.Printf("[%s] listening on: %v", who, ns.handlerBind.Address)
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
func (ns *NetService) gossipRandomAddresses() {
	for !ns.Stopping() {
		// wait for next turn
		ns.Sleep(GossipAddressInverval + time.Duration(rand.Intn(GossipAddressRandom))*time.Second)

		// choose a random node address
		nm, err := ns.store.ChooseNetNodeMsg()
		if err != nil {
			if !spec.IsNotFoundError(err) {
				log.Printf("[Node]: %v", err)
			}
			continue
		}

		// send the [Node][Addr] message to peers
		log.Printf("[Node]: gossiping a random peer address")
		msg := dnet.ReEncodeMessage(dnet.ChannelNode, node.TagAddress, (*[32]byte)(nm.PubKey), nm.Sig, nm.Payload)
		ns.forwardToPeers(msg)
	}
}

// goroutine
func (ns *NetService) findPeers() {
	who := "find-peers"
	for !ns.Stopping() {
		node, isNewPeer := ns.choosePeer(who) // blocking
		pubHex := hex.EncodeToString(node.PubKey[:])
		if node.IsValid() && !ns.havePeer(node.PubKey) && ns.lockPeer(node.PubKey) && !ns.isMyAddress(node) {
			log.Printf("[%s] choosing peer: %v [%v]", who, node.Addr, pubHex)
			// attempt to connect to the peer
			d := net.Dialer{Timeout: 30 * time.Second}
			conn, err := d.DialContext(ns.Context, "tcp", node.Addr.String())
			if err != nil {
				if isNewPeer {
					log.Printf("[%s] ADDED PEER dropped - connect failed: %v %v [%v]", who, err, node.Addr, pubHex)
				} else {
					log.Printf("[%s] connect failed: %v", who, err)
				}
			} else {
				peer := newPeer(conn, node.Addr, node.PubKey, true, true, ns) // outbound connection
				if ns.trackPeer(conn, peer, node.PubKey) {
					log.Printf("[%s] connected to peer (outbound): %v [%v]", who, node.Addr, pubHex)
					peer.start()
				} else { // already connected to peer, or Stop was called
					log.Printf("[%s] dropped peer, already connected (outbound): %v [%v]", who, node.Addr, pubHex)
					conn.Close()
					return
				}
			}
		} else if isNewPeer {
			// logging for explicitly added peers
			if !node.IsValid() {
				log.Printf("[%s] ADDED PEER ignored - not a valid address: %v [%v]", who, node.Addr, pubHex)
			} else if ns.havePeer(node.PubKey) {
				log.Printf("[%s] ADDED PEER ignored - already connected (same pubkey): %v [%v]", who, node.Addr, pubHex)
			} else if ns.isMyAddress(node) {
				log.Printf("[%s] ADDED PEER ignored - added this node's address (same ip:port): %v [%v]", who, node.Addr, pubHex)
			} else {
				log.Printf("[%s] ADDED PEER ignored - locked out for 30 sec (same pubkey): %v [%v]", who, node.Addr, pubHex)
			}
		}
		ns.Sleep(5 * time.Second) // slowly
	}
}

func (ns *NetService) isMyAddress(node spec.NodeInfo) bool {
	log.Printf("[%s] isMyAddress: %v", ns.ServiceName, node.Addr)
	log.Printf("[%s] bindAddrs: %v", ns.ServiceName, ns.bindAddrs)

	for _, addr := range ns.bindAddrs {
		if addr.Equal(node.Addr) {
			return true
		}
	}
	return false
}

// called from attractPeers
func (ns *NetService) choosePeer(who string) (addr spec.NodeInfo, isNewPeer bool) {
	for !ns.Stopping() {
		select {
		case np := <-ns.newPeers: // from ns.AddPeer()
			return np, true
		default:
			if ns.countPeers() < IdealPeers {
				ns.Sleep(5 * time.Second) // slowly
				np, err := ns.store.ChooseNetNode()
				if err != nil {
					if !spec.IsNotFoundError(err) {
						log.Printf("[%s] ChooseNetNode: %v", who, err)
					}
				} else {
					return np, false
				}
			}
		}
		// no peer available/required: sleep while receiving.
		select {
		case np := <-ns.newPeers: // from ns.AddPeer()
			return np, true
		case <-time.After(30 * time.Second):
			continue
		}
	}
	return spec.NodeInfo{}, false
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
		if ns.handlerBind.Network == "unix" {
			os.Remove(ns.handlerBind.Address)
		}
	}
	// close all active connections
	for _, c := range ns.connections {
		c.Close()
	}
}

// called from any
func (ns *NetService) forwardToPeers(msg dnet.RawMessage) {
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
			case hand.send <- dnet.RawMessage{Header: rawHdr, Payload: payload}:
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

// goroutine
func (ns *NetService) seedFromDNS() {
	who := "seed-from-dns"
	var seed_ips []net.IP
	for !ns.Stopping() {
		var err error
		seed_ips, err = net.LookupIP(SeedDNS)
		if err != nil {
			log.Printf("[%s] cannot resolve seed: %v: %v", who, SeedDNS, err)
		} else if len(seed_ips) > 0 {
			break
		}
		ns.Sleep(SeedAttemptTime + time.Duration(rand.Intn(SeedAttemptRandom))*time.Second)
	}
	log.Printf("[%s] resolved seeds: %v", who, seed_ips)
	for len(seed_ips) > 0 && !ns.Stopping() {
		// choose a random remaining ip address
		idx := rand.Intn(len(seed_ips))
		ip := seed_ips[idx]
		log.Printf("[%s] connecting to seed node: %v", who, ip)
		d := net.Dialer{Timeout: 30 * time.Second}
		remote := dnet.Address{Host: ip, Port: dnet.DogeNetDefaultPort}
		conn, err := d.DialContext(ns.Context, "tcp", remote.String())
		if err != nil {
			log.Printf("[%s] connect failed: %v", who, err)
		} else {
			peer := newPeer(conn, remote, NoPubKey, true, false, ns) // outbound connection
			if ns.trackPeer(conn, peer, NoPubKey) {
				log.Printf("[%s] seed node connected (outbound): %v", who, remote)
				// this peer will call adoptPeer once is receives the peer pubKey.
				peer.start()
			} else { // Stop was called
				log.Printf("[%s] dropped seed node, shutting down: %v", who, remote)
				conn.Close()
				return
			}
			// remove from unordered array
			seed_ips[idx] = seed_ips[len(seed_ips)-1]
			seed_ips = seed_ips[:len(seed_ips)-1]
		}
		// always proceed slowly
		ns.Sleep(SeedAttemptTime + time.Duration(rand.Intn(SeedAttemptRandom))*time.Second)
	}
}
