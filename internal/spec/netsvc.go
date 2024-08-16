package spec

import (
	"code.dogecoin.org/governor"
)

type NetSvc interface {
	governor.Service
	AddPeer(node NodeInfo)
}
