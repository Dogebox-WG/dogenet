package gossip

import (
	"github.com/dogeorg/dogenet/internal/gossip/service"
	"github.com/dogeorg/dogenet/internal/governor"
	"github.com/dogeorg/dogenet/internal/spec"
)

func New(address spec.Address, remotePort uint16, store spec.Store) governor.Service {
	return service.New(address, remotePort, store)
}
