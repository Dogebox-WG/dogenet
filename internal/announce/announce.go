package announce

import (
	"bytes"
	"encoding/hex"
	"log"
	"slices"
	"time"

	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/gossip/node"
	"code.dogecoin.org/governor"
)

const AnnounceLongevity = 24 * time.Hour
const QueueAnnouncement = 10 * time.Second

var ZeroOwner [32]byte

type Announce struct {
	governor.ServiceCtx
	_store       spec.Store
	store        spec.Store
	nodeKey      dnet.KeyPair          // node keypair for signing address messages
	changes      chan any              // input: changes to public address, owner pubkey, channels we have handlers for.
	receiver     spec.AnnounceReceiver // output: AnnounceReceiver receives new announcement RawMessages
	nextAnnounce node.AddressMsg       // current: public address, owner pubkey, channels, services
}

func New(public spec.Address, nodeKey dnet.KeyPair, store spec.Store, receiver spec.AnnounceReceiver, changes chan any) *Announce {
	return &Announce{
		_store:   store,
		nodeKey:  nodeKey,
		changes:  changes,
		receiver: receiver,
		nextAnnounce: node.AddressMsg{
			// Time is dynamically updated
			Address: public.Host.To16(),
			Port:    public.Port,
			Owner:   ZeroOwner[:], // announce zero-bytes unless we have an owner
			// Channels: are dynamically updated
			Services: []node.Service{
				// XXX this needs to be a config option (public Core Node address)
				{Tag: dnet.ServiceCore, Port: 22556},
			},
		},
	}
}

// goroutine
func (ns *Announce) Run() {
	ns.store = ns._store.WithCtx(ns.Context) // Service Context is first available here
	msg, remain, ok := ns.loadOrGenerateAnnounce()
	if ok {
		ns.receiver.ReceiveAnnounce(msg)
	}
	timer := time.NewTimer(remain)
	for !ns.Stopping() {
		select {
		case change := <-ns.changes:
			// whenever the node's address or channels change, gossip a new announcement.
			switch msg := change.(type) {
			case spec.ChangePublicAddress:
				ns.nextAnnounce.Address = msg.Addr.Host.To16()
				ns.nextAnnounce.Port = msg.Addr.Port
				log.Printf("[announce] received new public address: %v", msg.Addr)
			case spec.ChangeOwnerKey:
				newOwner := msg.Key[:]
				if bytes.Equal(ns.nextAnnounce.Owner, newOwner) {
					// ignore the message.
					// this avoids signing a new announcement early.
					continue
				}
				ns.nextAnnounce.Owner = msg.Key[:]
				log.Printf("[announce] received new owner: %v", hex.EncodeToString(msg.Key[:]))
				err := ns.store.SetAnnounceOwner(msg.Key[:])
				if err != nil {
					log.Printf("[announce] cannot store announcement owner: %v", err)
				}
			case spec.ChangeChannel:
				// query all currently-active channels.
				channels, err := ns.store.GetChannels()
				if err != nil {
					log.Printf("[announce] %v", err)
					continue
				}
				// check if channels are unchanged.
				// this avoids signing a new announcement early.
				current := ns.nextAnnounce.Channels
				if len(channels) == len(current) {
					slices.Sort(channels) // sort database result
					different := false
					for i, ch := range channels {
						if ch != current[i] {
							different = true
							break
						}
					}
					if !different {
						// ignore the message.
						continue
					}
				}
				ns.nextAnnounce.Channels = channels
				log.Printf("[announce] received new channels: %v", channels)
			default:
				log.Printf("[announce] invalid change message received")
				continue
			}
			// whenever a change is received, reset timer to 60 seconds.
			if ns.nextAnnounce.IsValid() {
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(QueueAnnouncement)
			}

		case <-timer.C:
			// every 24 hours, re-sign and gossip the announcement.
			msg, remain, ok := ns.generateAnnounce(ns.nextAnnounce)
			if ok {
				ns.receiver.ReceiveAnnounce(msg)
				log.Printf("[announce] sending announcement to all peers")
			}
			// restart the timer
			timer.Reset(remain)

		case <-ns.Context.Done():
			timer.Stop()
			return
		}
	}
}

func (ns *Announce) loadOrGenerateAnnounce() (raw dnet.RawMessage, rem time.Duration, ok bool) {
	// load stored channel list to include in our announcement
	// XXX load stored owner pubkey so we don't have to wait for Identity handler
	channels, err := ns.store.GetChannels()
	if err != nil {
		log.Printf("[announce] cannot load channels: %v", err)
		return dnet.RawMessage{}, AnnounceLongevity, false
	}
	slices.Sort(channels)               // sort database result
	ns.nextAnnounce.Channels = channels // initial set of channels
	// load the stored announcement from the database
	oldPayload, sig, expires, owner, err := ns.store.GetAnnounce()
	if err != nil {
		log.Printf("[announce] cannot load announcement: %v", err)
		return ns.generateAnnounce(ns.nextAnnounce)
	}
	if len(owner) == 32 {
		ns.nextAnnounce.Owner = owner // initial owner
	}
	if !ns.nextAnnounce.IsValid() {
		return dnet.RawMessage{}, AnnounceLongevity, false
	}
	now := time.Now().Unix()
	if len(oldPayload) >= node.AddrMsgMinSize && len(sig) == 64 && now < expires {
		// determine if the announcement we stored is the same as the announcement
		// we would produce now; if so, avoid signing and gossiping a new announcement
		oldMsg := node.DecodeAddrMsg(oldPayload) // for Time
		newMsg := ns.nextAnnounce                // copy
		newMsg.Time = oldMsg.Time                // ignore Time for Equals()
		if bytes.Equal(newMsg.Encode(), oldPayload) {
			// re-encode the stored announcement
			log.Printf("[announce] re-using stored announcement for %v seconds", expires-now)
			msg := dnet.ReEncodeMessage(node.ChannelNode, node.TagAddress, ns.nodeKey.Pub, sig, oldPayload)
			return msg, time.Duration(expires-now) * time.Second, true
		}
	}
	// create a new announcement and store it
	return ns.generateAnnounce(ns.nextAnnounce)
}

func (ns *Announce) generateAnnounce(newMsg node.AddressMsg) (raw dnet.RawMessage, rem time.Duration, ok bool) {
	if !newMsg.IsValid() {
		return dnet.RawMessage{}, AnnounceLongevity, false
	}

	// create and sign the new announcement.
	now := time.Now()
	newMsg.Time = dnet.UnixToDoge(now)
	log.Printf("[announce] signing a new announcement: %v", newMsg)
	payload := newMsg.Encode()
	msg := dnet.EncodeMessage(node.ChannelNode, node.TagAddress, ns.nodeKey, payload)

	// store the announcement to re-use on next startup.
	view := dnet.MsgView(msg)
	sig := view.Signature()[:]
	err := ns.store.SetAnnounce(payload, sig, now.Add(AnnounceLongevity).Unix())
	if err != nil {
		log.Printf("[announce] cannot store announcement: %v", err)
	}

	// update this node in the local database.
	// this makes the node visible to services on the local node.
	nodePub := ns.nodeKey.Pub[:]
	nodeAddr := spec.Address{Host: newMsg.Address, Port: newMsg.Port}
	time := newMsg.Time.Local().Unix()
	_, err = ns.store.AddNetNode(nodePub, nodeAddr, time, newMsg.Owner, newMsg.Channels, payload, sig)
	if err != nil {
		log.Printf("[announce] cannot store announcement: %v", err)
	}

	return dnet.RawMessage{Header: view.Header(), Payload: payload}, AnnounceLongevity, true
}
