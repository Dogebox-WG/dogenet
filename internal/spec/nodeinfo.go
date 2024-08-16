package spec

type NodeInfo struct {
	PubKey [32]byte
	Addr   Address
}

func (n NodeInfo) IsValid() bool {
	return n.Addr.IsValid()
}
