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

	"github.com/dogeorg/dogenet/pkg/msg"
	"github.com/dogeorg/dogenet/pkg/seeds"
)

type NodeAddressMap map[string]NodeInfo
type NodeInfo struct {
	Time     uint32
	Services msg.LocalNodeServices
}

type NetMapState struct {
	Nodes     NodeAddressMap // node address -> timstamp, services
	NewNodes  []string       // queue of newly discovered nodes (high priority)
	SeedNodes []string       // queue of seed nodes (low priority)
	migrated  bool           // true after migrateAddresses has run
}

type NetMap struct {
	mu     sync.Mutex
	state  NetMapState
	sample []string // a random sample of known nodes (not saved)
}

func NewNetMap() NetMap {
	return NetMap{state: NetMapState{Nodes: make(NodeAddressMap)}}
}

func (t *NetMap) Stats() (mapSize int, newNodes int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.state.Nodes), len(t.state.NewNodes)
}

type Payload struct {
	Address  string                `json:"address"`
	Time     uint32                `json:"time"`
	Services msg.LocalNodeServices `json:"services"`
}

func (t *NetMap) Payload() (res []Payload) {
	t.mu.Lock()
	defer t.mu.Unlock()
	res = make([]Payload, 0, len(Map.state.Nodes))
	for key, val := range Map.state.Nodes {
		res = append(res, Payload{
			Address:  key,
			Time:     val.Time,
			Services: val.Services,
		})
	}
	return
}

func (t *NetMap) Trim() {
	t.mu.Lock()
	defer t.mu.Unlock()
	// remove expired nodes from the map
	minKeep := time.Now().Add(-ExpiryTime).Unix()
	oldSize := len(Map.state.Nodes)
	newMap := make(NodeAddressMap, oldSize)
	for key, val := range Map.state.Nodes {
		if int64(val.Time) >= minKeep {
			newMap[key] = val
		}
	}
	Map.state.Nodes = newMap
	newSize := len(newMap)
	fmt.Printf("Trim expired nodes: %d expired, %d in Map\n", oldSize-newSize, newSize)
}

func (t *NetMap) AddNode(address net.IP, port uint16, time uint32, services msg.LocalNodeServices) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := net.JoinHostPort(address.String(), strconv.Itoa(int(port)))
	old, found := Map.state.Nodes[key]
	if !found || time > old.Time {
		// insert or replace
		Map.state.Nodes[key] = NodeInfo{
			Time:     time,
			Services: services,
		}
	}
	if !found {
		Map.state.NewNodes = append(Map.state.NewNodes, key)
	}
}

func (t *NetMap) UpdateTime(address string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		log.Printf("[UpdateTime] Cannot parse address: %v\n", address)
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if node, found := Map.state.Nodes[host]; found {
		node.Time = uint32(time.Now().Unix())
	}
}

func (t *NetMap) AddNewNode(address net.IP, port uint16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := net.JoinHostPort(address.String(), strconv.Itoa(int(port)))
	Map.state.NewNodes = append(Map.state.NewNodes, key)
}

func (t *NetMap) ChooseNode() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Printf("Queue: %d new, %d sampled, %d seeds, %d Map\n", len(t.state.NewNodes), len(t.sample), len(t.state.SeedNodes), len(t.state.Nodes))
	for {
		// highest priority: connect to newly discovered nodes.
		if len(t.state.NewNodes) > 0 {
			var addr string
			t.state.NewNodes, addr = pluckRandom(t.state.NewNodes)
			return addr
		}
		// next priority: connect to a random sample of known nodes.
		if len(t.sample) > 0 {
			var addr string
			t.sample, addr = pluckRandom(t.sample)
			return addr
		}
		// generate another sample of known nodes (XXX cull first)
		fmt.Println("Sampling the network map.")
		t.sampleNodeMap()
		if len(t.sample) > 0 {
			continue
		}
		// lowest priority: connect to seed nodes.
		if len(t.state.SeedNodes) > 0 {
			var addr string
			t.state.SeedNodes, addr = pluckRandom(t.state.SeedNodes)
			return addr
		}
		// fetch new seed nodes from DNS.
		// t.seedFromDNS()
		if len(t.state.SeedNodes) < 1 {
			t.seedFromFixed()
		}
	}
}

func pluckRandom(arr []string) ([]string, string) {
	len := len(arr)
	idx := rand.Intn(len)
	val := arr[idx]
	arr[idx] = arr[len-1] // copy down last elem
	arr = arr[:len-1]     // remove last elem
	return arr, val
}

func (t *NetMap) sampleNodeMap() {
	// choose 100 or so nodes at random from the nodeMap
	nodeMap := Map.state.Nodes
	mod := len(nodeMap) / 100
	if mod < 1 {
		mod = 1
	}
	idx := 0
	samp := rand.Intn(mod) // initial sample
	for key := range nodeMap {
		if idx >= samp {
			t.sample = append(t.sample, key)
			samp = idx + 1 + rand.Intn(mod) // next sample
		}
		idx++
	}
}

func (t *NetMap) seedFromDNS() {
	for _, node := range seeds.DNSSeeds {
		ips, err := net.LookupIP(node)
		if err != nil {
			fmt.Println("Error resolving DNS:", node, ":", err)
			continue
		}
		for _, ip := range ips {
			key := net.JoinHostPort(ip.String(), "22556")
			t.state.SeedNodes = append(t.state.SeedNodes, key)
			fmt.Println("Seed from DNS:", key)
		}
	}
}

func (t *NetMap) seedFromFixed() {
	for _, seed := range seeds.FixedSeeds {
		key := net.JoinHostPort(net.IP(seed.Host).String(), strconv.Itoa(seed.Port))
		t.state.SeedNodes = append(t.state.SeedNodes, key)
		fmt.Println("Seed from Fixture:", key)
	}
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
	if !t.state.migrated {
		migrateAddresses(&t.state)
		t.state.migrated = true
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
	newMap := make(NodeAddressMap, len(state.Nodes))
	for key, val := range state.Nodes {
		newMap[migrateAddress(key)] = val
	}
	state.Nodes = newMap
	// migrate NewNodes
	newNodes := make([]string, 0, len(state.NewNodes))
	for _, val := range state.NewNodes {
		newNodes = append(newNodes, migrateAddress(val))
	}
	state.NewNodes = newNodes
	// migrate SeedNodes
	seedNodes := make([]string, 0, len(state.NewNodes))
	for _, val := range state.SeedNodes {
		seedNodes = append(seedNodes, migrateAddress(val))
	}
	state.SeedNodes = seedNodes
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
