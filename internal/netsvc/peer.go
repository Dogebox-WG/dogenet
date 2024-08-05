package netsvc

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/gossip/node"

	"code.dogecoin.org/dogenet/internal/spec"
)

const OldestTime = -((30 * 24) + 1) * time.Hour // 30 days + 1 hour into the past
const NewestTime = 1 * time.Hour                // 1 hour into the future

// Peer connections exchange messages with a remote peer;
// forward received messages to channel owners,
// and periodically send gossip from channel owners.

type peerConn struct {
	ns       *NetService
	conn     net.Conn
	store    spec.StoreCtx
	receive  map[dnet.Tag4CC]chan dnet.Message
	send     chan []byte // raw message
	mutex    sync.Mutex
	addr     spec.Address // Peer's public address
	peerPub  dnet.PubKey  // Peer's pubkey (32 bytes) for outbound connections, nil for inbound connections
	nodeKey  dnet.KeyPair // [const] to sign `Addr` messages (key for THIS node)
	idenAddr spec.Address // [const] to include in `Addr` messages (address of THIS node)
	idenPub  dnet.PubKey  // [const] to include in `Addr` messages (pubkey of THIS node's owner)
}

func newPeer(conn net.Conn, addr spec.Address, peerPub dnet.PubKey, ns *NetService) *peerConn {
	peer := &peerConn{
		ns:       ns,
		conn:     conn,
		store:    ns.cstore,
		receive:  make(map[dnet.Tag4CC]chan dnet.Message),
		send:     make(chan []byte),
		addr:     addr,
		peerPub:  peerPub,
		nodeKey:  ns.nodeKey,
		idenAddr: ns.pubAddr,
		idenPub:  ns.idenPub,
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
	if peer.peerPub != nil {
		// MUST be an outbound connection, since we know the peer's PubKey.
		// 1. MUST announce THIS node's [Node][Addr] on outbound connections.
		peer.sendMyAddress(conn)
		// 2. Wait for the "return announcement" from the peer.
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive from peer: %v", who, err)
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
		// Verify that we connected to the expected peer.
		if !bytes.Equal(msg.PubKey, peer.peerPub) {
			log.Printf("[%s] connected to wrong peer: found PubKey %v but expected %v", who, hex.EncodeToString(msg.PubKey), hex.EncodeToString(peer.peerPub))
			peer.ns.closePeer(peer)
			return
		}
		// 5. Update the peer address, timestamp, etc in our database.
		// NB. This may broadcast a [Node][Addr] to other connected peers.
		newwho, err := peer.ingestAddress(msg)
		if err != nil {
			log.Printf("[%s] %v", who, err)
		} else {
			who = newwho
		}
		// 6. OK to start forwaring messages to the peer now.
		go peer.sendToPeer(who)
	} else {
		// MUST be an inbound connection.
		// 1. Wait for the [Node][Addr] announcement from the peer.
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// 2. Check if we received our own pubkey (connected to self)
		if bytes.Equal(msg.PubKey, peer.nodeKey.Pub) {
			log.Printf("[%s] connected to self: [%v]", who, hex.EncodeToString(msg.PubKey))
			peer.ns.closePeer(peer)
			return
		}
		// 3. Verify it is a [Node][Addr] message.
		if msg.Chan != node.ChannelNode || msg.Tag != node.TagAddress {
			log.Printf("[%s] expecting [Node][Addr] message but received: [%v][%v]", who, msg.Chan.String(), msg.Tag.String())
			peer.ns.closePeer(peer)
			return
		}
		// 4. Verify the peer announced a valid address.
		// NB. This may broadcast a [Node][Addr] to other connected peers.
		who, err := peer.ingestAddress(msg)
		if err != nil {
			log.Printf("[%s] %v", who, err)
		}
		// 5. Send our [Node][Addr] "return announcement"
		peer.sendMyAddress(conn)
		// 6. OK to start forwaring messages to the peer now.
		go peer.sendToPeer(who)
	}
	// Once peers have exchanged [Node][Addr] messages,
	// start relaying inbound messages to the protocol handlers.
	for !peer.ns.Stopping() {
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		if msg.Chan == node.ChannelNode && msg.Tag == node.TagAddress {
			// Received a [Node][Addr] announcement about some/any node.
			// NB. This may broadcast a [Node][Addr] to all connected peers,
			// excluding those still waiting for a "return announcement".
			if bytes.Equal(msg.PubKey, peer.nodeKey.Pub) {
				log.Printf("[%s] ignored my own announce: [%v]", who, hex.EncodeToString(msg.PubKey))
			} else {
				who, err = peer.ingestAddress(msg)
				if err != nil {
					log.Printf("[%s] %v", who, err)
				}
			}
		}
		// Forward the received message to channel owners.
		log.Printf("[%s] received from peer: %v %v", who, msg.Chan, msg.Tag)
		if !peer.ns.forwardToHandlers(msg.Chan, msg.RawMsg) {
			log.Printf("[%s] no handlers on channel: %s", who, msg.Chan)
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
	hexpub := hex.EncodeToString(msg.PubKey)
	log.Printf("received announce: %v:%v [%v]", ip.String(), addr.Port, hexpub)
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		// return "", fmt.Errorf("peer announced a private address: %v [%v]", ip.String(), hexpub)
		log.Printf("peer announced a private address: %v [%v]", ip.String(), hexpub)
	}
	// Check the timestamp: cannot be more than 30 days old, or more than 1 hour in the future.
	ts := addr.Time.Local()
	now := time.Now()
	if ts.Before(now.Add(OldestTime)) || ts.After(now.Add(NewestTime)) {
		return "", fmt.Errorf("peer timestamp out of range: %v vs %v (our time): %v [%v]", ts.String(), now.String(), ip.String(), hexpub)
	}
	// Add the peer to our database (update peer info for known peer)
	public := dnet.Address{Host: ip, Port: addr.Port}
	isnew, err := peer.store.AddNetNode(msg.PubKey, public, ts.Unix(), addr.Owner, addr.Channels, msg.Payload, msg.Signature)
	if err != nil {
		return
	}
	// Update `who` string for this peer.
	who = fmt.Sprintf("%v/%v", hex.EncodeToString(msg.PubKey[0:6]), public)
	// Lock peer fields and update.
	peer.mutex.Lock()
	defer peer.mutex.Unlock()
	peer.peerPub = msg.PubKey
	peer.addr = public
	if isnew {
		// re-broadcast the `Addr` message to all connected peers
		peer.ns.forwardToPeers(msg.RawMsg)
	}
	return
}

// goroutine
func (peer *peerConn) sendToPeer(who string) {
	conn := peer.conn
	for !peer.ns.Stopping() {
		select {
		case raw := <-peer.send:
			// forward the raw message to the peer
			cha, tag := dnet.MsgView(raw).ChanTag()
			log.Printf("[%s] sending to peer: %v %v", who, cha, tag)
			_, err := conn.Write(raw)
			if err != nil {
				log.Printf("[%s] failed to send: %v", who, err)
				peer.ns.closePeer(peer)
				return
			}
		case <-peer.ns.Context.Done():
			// shutting down
			peer.ns.closePeer(peer)
			return
		}
	}
}

// runs on receiveFromPeer
func (peer *peerConn) sendMyAddress(conn net.Conn) {
	addr := node.AddressMsg{
		Time:    dnet.DogeNow(),
		Address: peer.addr.Host.To16(), // nil edge-case if invalid
		Port:    peer.addr.Port,
		Channels: []dnet.Tag4CC{
			dnet.ChannelIdentity,
			dnet.ChannelB0rk,
		},
		Services: []node.Service{
			{Tag: dnet.ServiceCore, Port: 22556, Data: ""},
		},
	}
	addr.Owner = peer.idenPub // shared!
	conn.Write(dnet.EncodeMessage(dnet.ChannelNode, node.TagAddress, peer.nodeKey, addr.Encode()))
}
