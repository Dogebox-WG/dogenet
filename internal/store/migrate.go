package store

// type oldInfo struct {
// 	Time     uint32 // last time any other node connected to the node address
// 	Services uint64 // services from the node's version message
// }

// type oldIDMap map[string]oldInfo
// type oldNodeSet struct {
// 	Nodes     oldIDMap
// 	Sample    []string
// 	NewNodes  []string
// 	SeedNodes []string
// }
// type oldState struct {
// 	Core      oldNodeSet
// 	Net       oldNodeSet
// 	Nodes     oldIDMap // migrate -> Core.Nodes
// 	NewNodes  []string // migrate -> Core.NewNodes
// 	SeedNodes []string // migrate -> Core.SeedNodes
// 	migrated  int      // 0=slash-port-str 1=colon-port-str 2=spec.Address
// }

// func migrateToFormat3(path string) (NetMapState, error) {
// 	// re-read gob as old format
// 	old := oldState{
// 		Core: oldNodeSet{Nodes: make(oldIDMap)},
// 		Net:  oldNodeSet{Nodes: make(oldIDMap)},
// 	}
// 	new := NetMapState{
// 		Core: NodeSet{Nodes: make(nodeIDMap)},
// 		Net:  NodeSet{Nodes: make(nodeIDMap)},
// 	}
// 	err := readGobInto(path, &old)
// 	if err != nil {
// 		return new, err
// 	}
// 	// migrate from format 0 to format 1
// 	if old.migrated == 0 {
// 		migrateAddresses(&old)
// 	}
// 	// migrate from core-only to core-and-net
// 	if old.Nodes != nil {
// 		old.Core.Nodes = old.Nodes
// 	}
// 	if old.NewNodes != nil {
// 		old.Core.NewNodes = old.NewNodes
// 	}
// 	// migrate format 1 to format 2
// 	if old.Core.Nodes != nil {
// 		migrateMapOfAddresses(old.Core.Nodes, new.Core.Nodes)
// 	}
// 	// if old.Core.NewNodes != nil {
// 	// 	newNodes := migrateListOfAddresses(old.Core.NewNodes)
// 	// 	now := time.Now().Unix()
// 	// 	for _, addr := range newNodes {
// 	// 		new.Core.NewNodes = append(new.Core.NewNodes, NewNode{Addr: addr, Time: now})
// 	// 	}
// 	// }
// 	if old.Core.SeedNodes != nil {
// 		new.Core.SeedNodes = migrateListOfAddresses(old.Core.SeedNodes)
// 	}
// 	new.migrated = 3
// 	return new, nil
// }

// func migrateMapOfAddresses(from oldIDMap, to nodeIDMap) {
// 	for key, val := range from {
// 		addr, err := spec.ParseAddress(key)
// 		if err == nil { // only keep if valid
// 			to[addr.NodeID()] = NodeInfo{Address: addr, Services: val.Services, Time: int64(val.Time)}
// 		} else {
// 			log.Printf("Dropped bad address: %s\n", key)
// 		}
// 	}
// }

// func migrateListOfAddresses(array []string) []Address {
// 	result := make([]Address, 0, len(array))
// 	for _, val := range array {
// 		addr, err := spec.ParseAddress(val)
// 		if err == nil { // only keep if valid
// 			result = append(result, addr)
// 		} else {
// 			log.Printf("Dropped bad address: %s\n", val)
// 		}
// 	}
// 	return result
// }

// // migrateToFormat1
// func migrateAddresses(state *oldState) {
// 	// migrate map keys
// 	if state.Nodes != nil {
// 		newMap := make(oldIDMap, len(state.Nodes))
// 		for key, val := range state.Nodes {
// 			newMap[migrateNodeKey(string(key))] = val
// 		}
// 		state.Nodes = newMap
// 	}
// 	// migrate NewNodes
// 	if state.NewNodes != nil {
// 		newNodes := make([]string, 0, len(state.NewNodes))
// 		for _, val := range state.NewNodes {
// 			newNodes = append(newNodes, migrateNodeKey(string(val)))
// 		}
// 		state.NewNodes = newNodes
// 	}
// }

// func migrateNodeKey(id string) string {
// 	// convert from "ip/port" to net.Dial format
// 	if strings.Contains(string(id), "/") {
// 		elems := strings.Split(string(id), "/")
// 		ip, port := elems[0], elems[1]
// 		return net.JoinHostPort(ip, port)
// 	}
// 	return id
// }
