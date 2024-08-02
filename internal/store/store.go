package store

// Nodes need to be indexed by channel,
// so the peer can connect to enough nodes for each channel.

// When a given channel is rarely used on the network,
// the peer will end up seeking out those nodes to connect to.

// type NodeID = spec.NodeID
// type Address = spec.Address

// type nodeIDMap map[NodeID]NodeInfo

// type NodeInfo struct {
// 	Address  Address // current network address (IP:port)
// 	Time     int64   // last time any other node connected to the node address
// 	Services uint64  // services from the node's version message
// 	HasNet   bool    // true if the node's agent string contains @map (Core nodes only)
// }

// type NewNode struct {
// 	Addr Address
// 	Time int64
// }

// type NodeSet struct {
// 	Nodes     nodeIDMap // node ID -> address, timstamp, services
// 	Sample    []NewNode // random sample of known nodes (selected from Nodes)
// 	NewNodes  []NewNode // queue of newly discovered nodes (high priority)
// 	SeedNodes []Address // queue of seed nodes (low priority)
// }

// index: `id_32` (primary key unique pubkey lookup)
// index: `ip_16`,`port_2` (unique address lookup)
// new fields: `ident_32`,`sig_64`
// type NetMapState struct {
// 	NodePub  []byte
// 	NodePriv []byte
// 	Core     NodeSet
// 	Net      NodeSet
// 	migrated int // format: 0=slash-port 1=colon-port 2=spec.Address
// }

// type NetMap struct {
// 	mu    sync.Mutex
// 	state NetMapState // persisted in Gob file
// }

// var emptyPriv spec.PrivKey

// func New() spec.Store {
// 	return &NetMap{state: NetMapState{
// 		Core: NodeSet{Nodes: make(nodeIDMap)},
// 		Net:  NodeSet{Nodes: make(nodeIDMap)},
// 	}}
// }

// func (t *NetMap) NodeKey() (pub spec.PubKey, priv spec.PrivKey) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	if bytes.Equal(t.state.NodePriv[:], emptyPriv[:]) {
// 		pub, priv, err := ed25519.GenerateKey(cryptorand.Reader)
// 		if err != nil {
// 			panic(fmt.Sprintf("cannot generate pubkey: %v", err))
// 		}
// 		t.state.NodePub = pub
// 		t.state.NodePriv = priv.Seed()
// 	}
// 	return t.state.NodePub, t.state.NodePriv
// }

// func (t *NetMap) CoreStats() (mapSize int, newNodes int) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	return len(t.state.Core.Nodes), len(t.state.Core.NewNodes)
// }

// func (t *NetMap) NetStats() (mapSize int, newNodes int) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	return len(t.state.Net.Nodes), len(t.state.Net.NewNodes)
// }

// func (t *NetMap) NodeList() (res spec.NodeListRes) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	res.Core = make([]spec.CoreNode, 0, len(t.state.Core.Nodes))
// 	for _, val := range t.state.Core.Nodes {
// 		res.Core = append(res.Core, spec.CoreNode{
// 			Address:  val.Address.String(),
// 			Time:     val.Time,
// 			Services: val.Services,
// 		})
// 	}
// 	res.Net = make([]spec.NetNode, 0, len(t.state.Net.Nodes))
// 	for key, val := range t.state.Net.Nodes {
// 		res.Net = append(res.Net, spec.NetNode{
// 			PubKey:   key.String(),
// 			Address:  val.Address.String(),
// 			Time:     val.Time,
// 			Services: val.Services,
// 		})
// 	}
// 	return
// }

// func (t *NetMap) TrimNodes() {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	// remove expired nodes from the map
// 	minKeep := time.Now().Add(-spec.ExpiryTime).Unix()
// 	oldSize := len(t.state.Core.Nodes)
// 	newMap := make(nodeIDMap, oldSize)
// 	for key, val := range t.state.Core.Nodes {
// 		if val.Time >= minKeep {
// 			newMap[key] = val
// 		}
// 	}
// 	t.state.Core.Nodes = newMap
// 	newSize := len(newMap)
// 	fmt.Printf("Trim expired nodes: %d expired, %d in Map\n", oldSize-newSize, newSize)
// }

// func (t *NetMap) AddCoreNode(address Address, time int64, services uint64, hasNet bool) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	id := address.NodeID()
// 	old, found := t.state.Core.Nodes[id]
// 	if found && old.HasNet {
// 		hasNet = true // preserve HasNet when replacing
// 	}
// 	if !found || time > old.Time {
// 		// insert or replace
// 		t.state.Core.Nodes[id] = NodeInfo{
// 			Address:  address,
// 			Time:     time,
// 			Services: services,
// 			HasNet:   hasNet,
// 		}
// 	}
// 	if !found {
// 		t.state.Core.NewNodes = append(t.state.Core.NewNodes, NewNode{Addr: address, Time: time})
// 	}
// }

// func (t *NetMap) AddNetNode(pubkey spec.PubKey, address Address, time int64, services uint64) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	id := pubkey.NodeID()
// 	old, found := t.state.Net.Nodes[id]
// 	if !found || time > old.Time {
// 		// insert or replace
// 		t.state.Net.Nodes[id] = NodeInfo{
// 			Address:  address,
// 			Time:     time,
// 			Services: services,
// 		}
// 	}
// 	if !found {
// 		t.state.Net.NewNodes = append(t.state.Net.NewNodes, NewNode{Addr: address, Time: time})
// 	}
// }

// func (t *NetMap) UpdateCoreTime(address Address) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	if node, found := t.state.Core.Nodes[address.NodeID()]; found {
// 		node.Time = time.Now().Unix()
// 		t.state.Core.Nodes[address.NodeID()] = node // ugh
// 	}
// }

// func (t *NetMap) UpdateNetTime(key spec.PubKey) {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	if node, found := t.state.Net.Nodes[key.NodeID()]; found {
// 		node.Time = time.Now().Unix()
// 		t.state.Net.Nodes[key.NodeID()] = node // ugh
// 	}
// }

// func (t *NetMap) ChooseCoreNode() Address {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	fmt.Printf("Core: %d new, %d sampled, %d known\n", len(t.state.Core.NewNodes), len(t.state.Core.Sample), len(t.state.Core.Nodes))
// 	return chooseNode(&t.state.Core)
// }

// func (t *NetMap) ChooseNetNode() Address {
// 	t.mu.Lock()
// 	defer t.mu.Unlock()
// 	fmt.Printf("DNet: %d new, %d sampled, %d known\n", len(t.state.Net.NewNodes), len(t.state.Net.Sample), len(t.state.Net.Nodes))
// 	return chooseNode(&t.state.Net)
// }

// func pluckRandom(arr []NewNode) ([]NewNode, NewNode) {
// 	len := len(arr)
// 	idx := rand.Intn(len)
// 	val := arr[idx]
// 	arr[idx] = arr[len-1] // copy down last elem
// 	arr = arr[:len-1]     // remove last elem
// 	return arr, val
// }

// func sampleNodeMap(nodeMap nodeIDMap) (sample []NewNode) {
// 	// choose 100 or so nodes at random from the nodeMap
// 	mod := len(nodeMap) / 100
// 	if mod < 1 {
// 		mod = 1
// 	}
// 	idx := 0
// 	samp := rand.Intn(mod) // initial sample
// 	for _, val := range nodeMap {
// 		if idx >= samp {
// 			sample = append(sample, NewNode{Addr: val.Address, Time: val.Time})
// 			samp = idx + 1 + rand.Intn(mod) // next sample
// 		}
// 		idx++
// 	}
// 	return
// }

// func chooseNode(nodeSet *NodeSet) Address {
// 	// expire old addresses from the sets.
// 	keep := time.Now().Add(-spec.ExpiryTime).Unix()
// 	// highest priority: connect to newly discovered nodes.
// 	for len(nodeSet.NewNodes) > 0 {
// 		var addr NewNode
// 		nodeSet.NewNodes, addr = pluckRandom(nodeSet.NewNodes)
// 		if addr.Time >= keep {
// 			return addr.Addr
// 		}
// 	}
// 	// next priority: connect to a random sample of known nodes.
// 	for len(nodeSet.Sample) > 0 {
// 		var addr NewNode
// 		nodeSet.Sample, addr = pluckRandom(nodeSet.Sample)
// 		if addr.Time >= keep {
// 			return addr.Addr
// 		}
// 	}
// 	// generate another sample of known nodes (XXX cull first)
// 	nodeSet.Sample = sampleNodeMap(nodeSet.Nodes)
// 	fmt.Printf("Sampled %d nodes.\n", len(nodeSet.Sample))
// 	for len(nodeSet.Sample) > 0 {
// 		var addr NewNode
// 		nodeSet.Sample, addr = pluckRandom(nodeSet.Sample)
// 		if addr.Time >= keep {
// 			fmt.Printf("Kept %d samlpe nodes.\n", len(nodeSet.Sample))
// 			return addr.Addr
// 		}
// 	}
// 	fmt.Printf("Kept %d samlpe nodes.\n", len(nodeSet.Sample))
// 	// no nodes available
// 	return Address{}
// }
