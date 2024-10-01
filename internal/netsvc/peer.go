package netsvc

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/gossip/node"

	"code.dogecoin.org/dogenet/internal/spec"
)

const OldestAddrTime = -(30 * 24) * time.Hour // 30 days in the past
const NewestAddrTime = 5 * time.Minute        // 5 minutes into the future

// Peer connections exchange messages with a remote peer;
// forward received messages to channel owners,
// and periodically send gossip from channel owners.

type peerConn struct {
	ns         *NetService
	conn       net.Conn
	store      spec.StoreCtx
	allowLocal bool
	isOutbound bool
	hasPub     bool // has a peer pubkey
	receive    map[dnet.Tag4CC]chan dnet.Message
	send       chan dnet.RawMessage // raw message
	mutex      sync.Mutex
	addr       spec.Address // Peer's public address
	peerPub    [32]byte     // Peer's pubkey (pre-set for outbound, if known)
	nodeKey    dnet.KeyPair // [const] to sign `Addr` messages (key for THIS node)
}

func newPeer(conn net.Conn, addr spec.Address, peerPub [32]byte, outbound bool, hasPub bool, ns *NetService) *peerConn {
	peer := &peerConn{
		ns:         ns,
		conn:       conn,
		store:      ns.cstore,
		allowLocal: ns.allowLocal, // allow local IP address in Announcement messages (for local testing)
		isOutbound: outbound,
		hasPub:     hasPub,
		receive:    make(map[dnet.Tag4CC]chan dnet.Message),
		send:       make(chan dnet.RawMessage, 100),
		addr:       addr,
		peerPub:    peerPub,
		nodeKey:    ns.nodeKey,
	}
	return peer
}

func (peer *peerConn) start() {
	who := ""
	if len(peer.peerPub) >= 6 {
		who = fmt.Sprintf("%v/%v", hex.EncodeToString(peer.peerPub[0:6]), peer.addr.String())
	} else {
		who = fmt.Sprintf("%v", peer.addr.String())
	}
	go peer.receiveFromPeer(who)
}

// goroutine
func (peer *peerConn) receiveFromPeer(who string) {
	conn := peer.conn
	reader := bufio.NewReader(conn)
	if peer.isOutbound {
		// An outbound connection; we send the initial announce message.
		// 1. MUST announce THIS node's [Node][Addr] on outbound connections.
		err := peer.sendMyAddress(conn)
		if err != nil {
			log.Printf("[%s] failed to send [Node][Addr] to peer: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// 2. Wait for the "return announcement" from the peer.
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive return announcement: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// 3. MUST be a [Node][Addr] message announcing the peer.
		if msg.Chan != node.ChannelNode || msg.Tag != node.TagAddress {
			log.Printf("[%s] expecting [Node][Addr] message but received: [%v][%v]", who, msg.Chan.String(), msg.Tag.String())
			peer.ns.closePeer(peer)
			return
		}
		// 4. We ALWAYS know the PeerPub for outbound connections.
		// Except when connecting to DNS seed nodes.
		if peer.hasPub {
			if !bytes.Equal(msg.PubKey, peer.peerPub[:]) {
				log.Printf("[%s] connected to wrong peer: found PubKey %v but expected %v", who, hex.EncodeToString(msg.PubKey), hex.EncodeToString(peer.peerPub[:]))
				peer.ns.closePeer(peer)
				return
			}
		} else {
			copy(peer.peerPub[:], msg.PubKey)
			who = fmt.Sprintf("%v/%v", hex.EncodeToString(peer.peerPub[0:6]), peer.addr.String())
			// Check if we're already connected to this peer
			// Only call this if we started with NoPubKey (hasPub == false)
			if !peer.ns.adoptPeer(peer, peer.peerPub) {
				log.Printf("[%s] already connected to peer: [%v] (inbound connection)", who, hex.EncodeToString(msg.PubKey))
				peer.ns.closePeer(peer)
				return
			}
		}
		// 5. Check if we received our own pubkey (connected to self)
		if bytes.Equal(msg.PubKey, peer.nodeKey.Pub[:]) {
			log.Printf("[%s] connected to self: [%v] (outbound connection)", who, hex.EncodeToString(msg.PubKey))
			peer.ns.closePeer(peer)
			return
		}
		// 6. Update the peer address, timestamp, etc in our database.
		// NB. This may broadcast a [Node][Addr] to other connected peers.
		newwho, err := peer.ingestAddress(msg)
		if err != nil {
			log.Printf("[%s] %v", who, err)
			peer.ns.closePeer(peer)
			return
		} else {
			who = newwho
		}
		// 7. OK to start forwaring messages to the peer now.
		go peer.sendToPeer(who)
	} else {
		// MUST be an inbound connection.
		// 1. Wait for the [Node][Addr] announcement from the peer.
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive first inbound message: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		copy(peer.peerPub[:], msg.PubKey)
		who = fmt.Sprintf("%v/%v", hex.EncodeToString(peer.peerPub[0:6]), peer.addr.String())
		// 2. Check if we received our own pubkey (connected to self)
		if bytes.Equal(msg.PubKey, peer.nodeKey.Pub[:]) {
			log.Printf("[%s] connected to self: [%v] (inbound connection)", who, hex.EncodeToString(msg.PubKey))
			peer.ns.closePeer(peer)
			return
		}
		// 3. Check if we're already connected to this peer
		if !peer.ns.adoptPeer(peer, peer.peerPub) {
			log.Printf("[%s] already connected to peer: [%v] (inbound connection)", who, hex.EncodeToString(msg.PubKey))
			peer.ns.closePeer(peer)
			return
		}
		// 4. Verify it is a [Node][Addr] message.
		if msg.Chan != node.ChannelNode || msg.Tag != node.TagAddress {
			log.Printf("[%s] expecting [Node][Addr] message but received: [%v][%v] (inbound connection)", who, msg.Chan.String(), msg.Tag.String())
			peer.ns.closePeer(peer)
			return
		}
		// 5. Verify the peer announced a valid address.
		// NB. This may broadcast a [Node][Addr] to other connected peers.
		who, err := peer.ingestAddress(msg)
		if err != nil {
			log.Printf("[%s] %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// 6. Send our [Node][Addr] "return announcement"
		err = peer.sendMyAddress(conn)
		if err != nil {
			log.Printf("[%s] failed to send [Node][Addr] to peer: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// 7. OK to start forwaring messages to the peer now.
		go peer.sendToPeer(who)
	}
	// Once peers have exchanged [Node][Addr] messages,
	// start relaying inbound messages to the protocol handlers.
	for !peer.ns.Stopping() {
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			if strings.Contains(err.Error(), "signature") {
				log.Printf("[%s] failed to receive from peer: badness: %v", who, err)
			}
			log.Printf("[%s] failed to receive from peer: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		log.Printf("[%s] received from peer: [%v][%v]", who, msg.Chan, msg.Tag)
		if msg.Chan == node.ChannelNode {
			if msg.Tag == node.TagAddress {
				// Received a [Node][Addr] announcement about some/any node.
				// NB. This may broadcast a [Node][Addr] to all connected peers,
				// excluding those still waiting for a "return announcement".
				if bytes.Equal(msg.PubKey, peer.nodeKey.Pub[:]) {
					log.Printf("[%s] ignored my own announce: [%v]", who, hex.EncodeToString(msg.PubKey))
				} else {
					who, err = peer.ingestAddress(msg)
					if err != nil {
						log.Printf("[%s] %v", who, err)
						peer.ns.closePeer(peer)
						return
					}
				}
			} else {
				log.Printf("[%s] ignored unknown [Node] message: [%v]", who, msg.Tag)
			}
		} else {
			// Forward the received message to channel owners.
			if !peer.ns.forwardToHandlers(msg.Chan, msg.RawHdr, msg.Payload) {
				log.Printf("[%s] no handlers on channel: %s", who, msg.Chan)
			}
		}
	}
}

// runs in receiveFromPeer
func (peer *peerConn) ingestAddress(msg dnet.Message) (who string, err error) {
	defer func() {
		if e := recover(); e != nil { // for DecodeAddrMsg
			err = fmt.Errorf("address decode error: %v", e)
		}
	}()
	// Check that the peer address is a public IP address
	addr := node.DecodeAddrMsg(msg.Payload)
	ip := net.IP(addr.Address)
	peerAddr := dnet.Address{Host: ip, Port: addr.Port}
	hexpub := hex.EncodeToString(msg.PubKey)
	log.Printf("received announce: %v [%v]", peerAddr, hexpub)
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		if peer.allowLocal {
			log.Printf("peer announced a private address: %v [%v] (allowed via --local=true)", peerAddr, hexpub)
		} else {
			return "", fmt.Errorf("peer announced a private address: %v [%v]", peerAddr, hexpub)
		}
	}
	// Check the timestamp: cannot be older than the expiry time, or too far into the future.
	ts := addr.Time.Local()
	now := time.Now()
	if ts.Before(now.Add(OldestAddrTime)) || ts.After(now.Add(NewestAddrTime)) {
		return "", fmt.Errorf("peer timestamp out of range: %v vs %v (our time): %v [%v]", ts.String(), now.String(), peerAddr, hexpub)
	}
	// Update peer address and `who` string.
	who = fmt.Sprintf("%v/%v", hex.EncodeToString(msg.PubKey[0:6]), peerAddr)
	peer.setPeerAddress(peerAddr)
	// Add the peer to our database (update peer info for known peer)
	log.Printf("[%s] adding node: %v %v", who, peerAddr, hexpub)
	isnew, err := peer.store.AddNetNode(msg.PubKey, peerAddr, ts.Unix(), addr.Owner, addr.Channels, msg.Payload, msg.Signature)
	if isnew {
		// re-broadcast the `Addr` message to all connected peers
		peer.ns.forwardToPeers(dnet.RawMessage{Header: msg.RawHdr, Payload: msg.Payload})
	}
	return
}

func (peer *peerConn) setPeerAddress(peerAddr dnet.Address) {
	peer.mutex.Lock()
	defer peer.mutex.Unlock()
	peer.addr = peerAddr
}

// goroutine
func (peer *peerConn) sendToPeer(who string) {
	conn := peer.conn
	for !peer.ns.Stopping() {
		select {
		case raw := <-peer.send:
			// forward the raw message to the peer
			cha, tag := dnet.MsgView(raw.Header).ChanTag()
			log.Printf("[%s] sending to peer: [%v][%v]", who, cha, tag)
			_, err := conn.Write(raw.Header)
			if err != nil {
				log.Printf("[%s] failed to send: %v", who, err)
				peer.ns.closePeer(peer)
				return
			}
			_, err = conn.Write(raw.Payload)
			if err != nil {
				log.Printf("[%s] failed to send: %v", who, err)
				peer.ns.closePeer(peer)
				return
			}
		case <-peer.ns.Context.Done():
			// shutting down
			// no race: peer.peerPub is final before sendToPeer starts (closePeer OK to read peer.peerPub)
			peer.ns.closePeer(peer)
			return
		}
	}
}

// runs on receiveFromPeer
func (peer *peerConn) sendMyAddress(conn net.Conn) error {
	// XXX will need to block, unless NetService waits for
	// an Announce before listening for peers.
	msg := peer.ns.GetAnnounce()
	_, err := conn.Write(msg.Header)
	if err != nil {
		return err
	}
	_, err = conn.Write(msg.Payload)
	return err
}
