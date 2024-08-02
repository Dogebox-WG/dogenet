package netsvc

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net"
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
	addr     spec.Address
	store    spec.Store
	receive  map[dnet.Tag4CC]chan dnet.Message
	send     chan dnet.Message
	nodeKey  dnet.KeyPair // to sign `Addr` messages (key for THIS node)
	idenAddr spec.Address // to include in `Addr` messages (address of THIS node)
	idenPub  dnet.PubKey  // to include in `Addr` messages (pubkey of THIS node's owner)
	peerPub  dnet.PubKey  // Peer's pubkey (32 bytes) for outbound connections, nil for inbound connections
}

func newPeer(conn net.Conn, addr spec.Address, peerPub dnet.PubKey, ns *NetService) *peerConn {
	peer := &peerConn{
		ns:       ns,
		conn:     conn,
		addr:     addr,
		peerPub:  peerPub,
		store:    ns.store,
		receive:  make(map[dnet.Tag4CC]chan dnet.Message),
		send:     make(chan dnet.Message),
		nodeKey:  ns.nodeKey,
		idenAddr: ns.pubAddr,
		idenPub:  ns.idenPub,
	}
	return peer
}

func (peer *peerConn) start() {
	who := fmt.Sprintf("%v|%v", hex.EncodeToString(peer.peerPub[0:6]), peer.addr.String())
	go peer.receiveFromPeer(who)
	go peer.sendToPeer(who)
}

// goroutine
func (peer *peerConn) receiveFromPeer(who string) {
	conn := peer.conn
	reader := bufio.NewReader(conn)
	if peer.peerPub != nil {
		// MUST be an outbound connection, since we know the peer's PubKey.
		// MUST announce THIS node's address on outbound connections.
		peer.sendMyAddress(conn)
		// Wait for the Address announcement from the peer.
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive from peer: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// MUST be an `Addr` message.
		if msg.Chan != node.ChannelNode || msg.Tag != node.TagAddress || msg.Size != node.AddrMsgSize {
			log.Printf("[%s] expecting [Node][Addr] message but received: [%v][%v]", who, msg.Chan.String(), msg.Tag.String())
			peer.ns.closePeer(peer)
			return
		}
		// We ALWAYS know the PeerPub for outbound connections.
		// Verify that we connected to the expected peer.
		if !bytes.Equal(msg.PubKey, peer.peerPub) {
			log.Printf("[%s] connected to wrong peer: found PubKey %v but expected %v", who, hex.EncodeToString(msg.PubKey), hex.EncodeToString(peer.peerPub))
			peer.ns.closePeer(peer)
			return
		}
	} else {
		// MUST be an inbound connection.
		// Wait for the Address announcement from the peer.
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// MUST be an `Addr` message.
		if msg.Chan != node.ChannelNode || msg.Tag != node.TagAddress || msg.Size != node.AddrMsgSize {
			log.Printf("[%s] expecting [Node][Addr] message but received: [%v][%v]", who, msg.Chan.String(), msg.Tag.String())
			peer.ns.closePeer(peer)
			return
		}
		// Check that the peer address is a public IP address
		addr := node.DecodeAddrMsg(msg.Payload)
		ip := net.IP(addr.Address)
		if ip.IsGlobalUnicast() || ip.IsPrivate() {
			log.Printf("[%s] peer announced a private address: %v", who, hex.EncodeToString(peer.peerPub))
			peer.ns.closePeer(peer)
			return
		}
		// Check the timestamp: cannot be more than 30 days old, or more than 1 hour in the future.
		ts := addr.Time.Local()
		now := time.Now()
		if ts.Before(now.Add(OldestTime)) || ts.After(now.Add(NewestTime)) {
			log.Printf("[%s] peer timestamp out of range: %v vs %v (our time)", who, ts.String(), now.String())
			peer.ns.closePeer(peer)
			return
		}
		// Record the peer's PubKey
		peer.peerPub = msg.PubKey
		who = fmt.Sprintf("%v|%v", hex.EncodeToString(peer.peerPub[0:6]), peer.addr.String())
		// Add the peer to our database (update the timestamp for known peers)
		remote := spec.Address{Host: ip, Port: addr.Port}
		peer.store.AddNetNode(msg.PubKey, remote, ts.Unix(), addr.Channels, msg.RawMsg)
	}
	for !peer.ns.Stopping() {
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] failed to receive: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// forward received message to channel owners
		log.Printf("[%s] received from peer: %v %v", who, msg.Chan, msg.Tag)
		if !peer.ns.forwardToHandlers(msg) {
			log.Printf("[%s] no handlers on channel: %s", who, msg.Chan)
		}
	}
}

// goroutine
func (peer *peerConn) sendToPeer(who string) {
	conn := peer.conn
	for !peer.ns.Stopping() {
		select {
		case msg := <-peer.send:
			// forward the raw message to the peer
			log.Printf("[%s] sending to peer: %v %v", who, msg.Chan, msg.Tag)
			err := dnet.ForwardMessage(conn, msg)
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

func (peer *peerConn) sendMyAddress(conn net.Conn) {
	addr := node.AddressMsg{
		Time:    dnet.DogeNow(),
		Address: peer.addr.Host.To16(), // can return nil
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
	conn.Write(dnet.EncodeMessage(dnet.ChannelDoge, node.TagAddress, peer.nodeKey, addr.Encode()))
}
