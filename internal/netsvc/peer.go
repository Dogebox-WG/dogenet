package netsvc

import (
	"bufio"
	"log"
	"net"

	"rad/gossip/dnet"
	"rad/gossip/node"

	"github.com/dogeorg/dogenet/internal/spec"
)

// Peer connections exchange messages with a remote peer;
// forward received messages to channel owners,
// and periodically send gossip from channel owners.

type peerConn struct {
	ns       *netService
	conn     net.Conn
	address  spec.Address
	store    spec.Store
	receive  map[dnet.Tag4CC]chan dnet.Message
	send     chan dnet.Message
	signKey  dnet.PrivKey
	identPub dnet.PubKey
}

func newPeer(conn net.Conn, address spec.Address, ns *netService, priv dnet.PrivKey, pub dnet.PubKey) *peerConn {
	peer := &peerConn{
		ns:       ns,
		conn:     conn,
		address:  address,
		store:    ns.store,
		receive:  make(map[dnet.Tag4CC]chan dnet.Message),
		send:     make(chan dnet.Message),
		signKey:  priv,
		identPub: pub,
	}
	return peer
}

func (peer *peerConn) start() {
	go peer.receiveFromPeer()
	go peer.sendToPeer()
}

// goroutine
func (peer *peerConn) receiveFromPeer() {
	who := peer.address.String()
	conn := peer.conn
	reader := bufio.NewReader(conn)
	peer.sendMyAddress(conn)
	for !peer.ns.Stopping() {
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] cannot receive from peer: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// forward received message to channel owners
		log.Printf("[%s] received from peer: %v %v", who, msg.Chan, msg.Tag)
		if !peer.ns.forwardToHandlers(msg) {
			// send a reject on the channel, with message tag as data
			_, err := conn.Write(
				dnet.EncodeMessage(msg.Chan, node.TagReject, peer.signKey,
					node.EncodeReject(node.REJECT_CHAN, "", msg.Tag.Bytes())))
			if err != nil {
				log.Printf("[%s] failed to send reject: %s", who, msg.Tag)
				peer.ns.closePeer(peer)
				return
			}
		}
	}
}

// goroutine
func (peer *peerConn) sendToPeer() {
	who := peer.address.String()
	conn := peer.conn
	for !peer.ns.Stopping() {
		select {
		case msg := <-peer.send:
			// forward the raw message to the peer
			log.Printf("[%s] sending to peer: %v %v", who, msg.Chan, msg.Tag)
			err := dnet.ForwardMessage(conn, msg)
			if err != nil {
				log.Printf("[%s] cannot send to peer: %v", who, err)
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
		Address: peer.address.Host.To16(), // can return nil
		Port:    peer.address.Port,
		Channels: []dnet.Tag4CC{
			dnet.ChannelIdentity,
			dnet.ChannelB0rk,
		},
		Services: []node.Service{
			{Tag: dnet.ServiceCore, Port: 22556, Data: ""},
		},
	}
	addr.Owner = peer.identPub // shared!
	conn.Write(dnet.EncodeMessage(dnet.ChannelDoge, node.TagAddress, peer.signKey, addr.Encode()))
}
