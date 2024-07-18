package service

import (
	"bufio"
	"log"
	"net"

	"github.com/dogeorg/dogenet/internal/gossip/protocol"
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
	receive  map[protocol.Tag4CC]chan protocol.Message
	send     chan protocol.Message
	signKey  protocol.PrivKey
	identPub protocol.PubKey
}

func newPeer(conn net.Conn, address spec.Address, ns *netService, priv protocol.PrivKey, pub protocol.PubKey) *peerConn {
	peer := &peerConn{
		ns:       ns,
		conn:     conn,
		address:  address,
		store:    ns.store,
		receive:  make(map[protocol.Tag4CC]chan protocol.Message),
		send:     make(chan protocol.Message),
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
		msg, err := protocol.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] cannot receive from peer: %v", who, err)
			peer.ns.closePeer(peer)
			return
		}
		// forward received message to channel owners
		if !peer.ns.forwardToHandlers(msg) {
			// send a reject on the channel, with message tag as data
			_, err := conn.Write(
				protocol.EncodeMessage(msg.Chan, protocol.TagReject, peer.signKey,
					protocol.EncodeReject(protocol.REJECT_CHAN, "", msg.Tag.Bytes())))
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
			err := protocol.ForwardMessage(conn, msg)
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
	addr := protocol.AddressMsg{
		Time:    protocol.DogeNow(),
		Address: peer.address.Host.To16(), // can return nil
		Port:    peer.address.Port,
		Channels: []protocol.Tag4CC{
			protocol.ChannelIdentity,
			protocol.ChannelB0rk,
		},
		Services: []protocol.Service{
			{Tag: protocol.ServiceCore, Port: 22556, Data: ""},
		},
	}
	addr.Owner = peer.identPub // shared!
	conn.Write(protocol.EncodeMessage(protocol.ChannelDoge, protocol.TagAddress, peer.signKey, addr.Encode()))
}
