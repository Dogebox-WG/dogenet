package spec

import "code.dogecoin.org/gossip/dnet"

type AnnounceReceiver interface {
	ReceiveAnnounce(announce RawMessage)
}

type ChangePublicAddress struct {
	Addr Address
}

type ChangeOwnerKey struct {
	Key dnet.PubKey
}

type ChangeChannel struct {
	Tag    dnet.Tag4CC
	Remove bool
}
