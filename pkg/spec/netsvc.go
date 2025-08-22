package spec

import (
	"code.dogecoin.org/governor"
)

type NetSvc interface {
	governor.Service
	AnnounceReceiver
	AddPeer(node NodeInfo)
}
