package spec

type NodeInfo struct {
	PubKey []byte // 32 bytes
	Addr   Address
}

func (n NodeInfo) IsValid() bool {
	return len(n.PubKey) == 32 && n.Addr.IsValid()
}
