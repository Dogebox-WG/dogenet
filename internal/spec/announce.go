package spec

import "code.dogecoin.org/gossip/dnet"

type AnnounceReceiver interface {
	ReceiveAnnounce(announce dnet.RawMessage)
}

type ChangePublicAddress struct {
	Addr Address
}

type ChangeOwnerKey struct {
	Key dnet.PubKey
}

type ChangeChannel struct {
	Chan dnet.Tag4CC
}
