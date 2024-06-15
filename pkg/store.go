package dogenet

import (
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dogeorg/dogenet/internal/spec"
)

type NodeAddressMap map[string]NodeInfo
type NodeInfo struct {
	Time     uint32 // last time any other node connected to the node address
	Services uint64 // services from the node's version message
	IsNet    bool   // true if the node's agent string contains @map (Core nodes only)
}

type NodeSet struct {
	Nodes     NodeAddressMap // node address -> timstamp, services
	Sample    []string       // random sample of known nodes (selected from Nodes)
	NewNodes  []string       // queue of newly discovered nodes (high priority)
	SeedNodes []string       // queue of seed nodes e.g. from DNS (lowest priority)
}

type NetMapState struct {
	Core NodeSet
	Net  NodeSet
	// for Gob migration:
	Nodes     NodeAddressMap // migrate -> Core.Nodes
	NewNodes  []string       // migrate -> Core.NewNodes
	SeedNodes []string       // migrate -> Core.SeedNodes
	migrated  bool           // migrate old address format
}

type NetMap struct {
	mu    sync.Mutex
	state NetMapState // persisted in Gob file
}

func NewNetMap() NetMap {
	return NetMap{state: NetMapState{
		Core: NodeSet{Nodes: make(NodeAddressMap)},
		Net:  NodeSet{Nodes: make(NodeAddressMap)},
	}}
}

func (t *NetMap) CoreStats() (mapSize int, newNodes int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.state.Core.Nodes), len(t.state.Core.NewNodes)
}

func (t *NetMap) NetStats() (mapSize int, newNodes int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.state.Net.Nodes), len(t.state.Net.NewNodes)
}

func (t *NetMap) NodeList() (res []spec.Payload) {
	t.mu.Lock()
	defer t.mu.Unlock()
	res = make([]spec.Payload, 0, len(Map.state.Core.Nodes))
	for key, val := range Map.state.Core.Nodes {
		res = append(res, spec.Payload{
			Address:  key,
			Time:     val.Time,
			Services: val.Services,
		})
	}
	return
}

func (t *NetMap) TrimNodes() {
	t.mu.Lock()
	defer t.mu.Unlock()
	// remove expired nodes from the map
	minKeep := time.Now().Add(-spec.ExpiryTime).Unix()
	oldSize := len(Map.state.Core.Nodes)
	newMap := make(NodeAddressMap, oldSize)
	for key, val := range Map.state.Core.Nodes {
		if int64(val.Time) >= minKeep {
			newMap[key] = val
		}
	}
	Map.state.Core.Nodes = newMap
	newSize := len(newMap)
	fmt.Printf("Trim expired nodes: %d expired, %d in Map\n", oldSize-newSize, newSize)
}

func (t *NetMap) AddCoreNode(address net.IP, port uint16, time uint32, services uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := net.JoinHostPort(address.String(), strconv.Itoa(int(port)))
	old, found := Map.state.Core.Nodes[key]
	if !found || time > old.Time {
		// insert or replace
		Map.state.Core.Nodes[key] = NodeInfo{
			Time:     time,
			Services: services,
		}
	}
	if !found {
		Map.state.Core.NewNodes = append(Map.state.Core.NewNodes, key)
	}
}

func (t *NetMap) AddNetNode(address string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// untrusted and untried new address
	Map.state.Net.NewNodes = append(Map.state.Net.NewNodes, address)
}

func (t *NetMap) UpdateCoreTime(address string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		log.Printf("[UpdateTime] Cannot parse address: %v\n", address)
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if node, found := Map.state.Core.Nodes[host]; found {
		node.Time = uint32(time.Now().Unix())
	}
}

func (t *NetMap) ChooseCoreNode() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Printf("Core: %d new, %d sampled, %d known\n", len(t.state.Core.NewNodes), len(t.state.Core.Sample), len(t.state.Core.Nodes))
	fmt.Printf("DNet: %d new, %d sampled, %d known\n", len(t.state.Net.NewNodes), len(t.state.Net.Sample), len(t.state.Net.Nodes))
	return chooseNode(&t.state.Core)
}

func pluckRandom(arr []string) ([]string, string) {
	len := len(arr)
	idx := rand.Intn(len)
	val := arr[idx]
	arr[idx] = arr[len-1] // copy down last elem
	arr = arr[:len-1]     // remove last elem
	return arr, val
}

func sampleNodeMap(nodeMap NodeAddressMap) (coreSample []string) {
	// choose 100 or so nodes at random from the nodeMap
	mod := len(nodeMap) / 100
	if mod < 1 {
		mod = 1
	}
	idx := 0
	samp := rand.Intn(mod) // initial sample
	for key := range nodeMap {
		if idx >= samp {
			coreSample = append(coreSample, key)
			samp = idx + 1 + rand.Intn(mod) // next sample
		}
		idx++
	}
	return
}

func chooseNode(nodeSet *NodeSet) string {
	// highest priority: connect to newly discovered nodes.
	if len(nodeSet.NewNodes) > 0 {
		var addr string
		nodeSet.NewNodes, addr = pluckRandom(nodeSet.NewNodes)
		return addr
	}
	// next priority: connect to a random sample of known nodes.
	if len(nodeSet.Sample) > 0 {
		var addr string
		nodeSet.Sample, addr = pluckRandom(nodeSet.Sample)
		return addr
	}
	// generate another sample of known nodes (XXX cull first)
	fmt.Println("Sampling the network map.")
	nodeSet.Sample = sampleNodeMap(nodeSet.Nodes)
	if len(nodeSet.Sample) > 0 {
		var addr string
		nodeSet.Sample, addr = pluckRandom(nodeSet.Sample)
		return addr
	}
	// no nodes available
	return ""
}

func (t *NetMap) ReadGob(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file %q: %w", path, err)
	}
	defer file.Close()
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&t.state); err != nil {
		if err == io.EOF {
			return fmt.Errorf("file %q is empty", path)
		}
		return fmt.Errorf("cannot decode object from file %q: %w", path, err)
	}
	// migrate from old address format
	if !t.state.migrated {
		migrateAddresses(&t.state)
		t.state.migrated = true
	}
	// migrate from core-only format
	if t.state.Nodes != nil {
		t.state.Core.Nodes = t.state.Nodes
		t.state.Nodes = nil
	}
	if t.state.NewNodes != nil {
		t.state.Core.NewNodes = t.state.NewNodes
		t.state.NewNodes = nil
	}
	return nil
}

func (t *NetMap) WriteGob(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	tempFile, err := os.CreateTemp("", "temp_gob_file")
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(t.state); err != nil {
		return fmt.Errorf("cannot encode object: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file: %w", err)
	}
	if err := os.Rename(tempFile.Name(), path); err != nil {
		return fmt.Errorf("cannot rename temporary file to %q: %w", path, err)
	}
	return nil
}

func migrateAddresses(state *NetMapState) {
	// migrate map keys
	if state.Nodes != nil {
		newMap := make(NodeAddressMap, len(state.Nodes))
		for key, val := range state.Nodes {
			newMap[migrateAddress(key)] = val
		}
		state.Nodes = newMap
	}
	// migrate NewNodes
	if state.NewNodes != nil {
		newNodes := make([]string, 0, len(state.NewNodes))
		for _, val := range state.NewNodes {
			newNodes = append(newNodes, migrateAddress(val))
		}
		state.NewNodes = newNodes
	}
}

func migrateAddress(key string) string {
	// convert from "ip/port" to net.Dial format
	if strings.Contains(key, "/") {
		elems := strings.Split(key, "/")
		ip, port := elems[0], elems[1]
		return net.JoinHostPort(ip, port)
	}
	return key
}
