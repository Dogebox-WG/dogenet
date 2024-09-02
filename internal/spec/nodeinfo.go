package spec

type NodeInfo struct {
	PubKey [32]byte // array to be used as map key
	Addr   Address
}

func (n NodeInfo) IsValid() bool {
	return n.Addr.IsValid()
}

type NodeRecord struct {
	PubKey  []byte
	Payload []byte
	Sig     []byte
}

func (n NodeRecord) IsValid() bool {
	return len(n.Payload) > 0
}
