package dogenet

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/dogeorg/dogenet/internal/core/collector"
	"github.com/dogeorg/dogenet/internal/governor"
	"github.com/dogeorg/dogenet/internal/web"
)

const GobFilePath = "netmap.gob"

const DogeNetConnections = 4
const CoreNodeListeners = 0

var Map NetMap // persistent network map

func DogeNet(localNode string) {
	// load the previously saved state.
	Map = NewNetMap()
	err := Map.ReadGob(GobFilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Println("cannot read Gob file:", err)
		os.Exit(1)
	}

	gov := governor.New().CatchSignals().Restart(1 * time.Second)

	// periodically save the network map.
	gov.Add("map-saver", NewMapSaver())

	// stay connected to local node if specified.
	if localNode != "" {
		addr := net.ParseIP(localNode)
		if addr == nil {
			log.Println("Invalid ip address on command line:", localNode)
			os.Exit(1)
		}
		gov.Add("local-node", collector.New(&Map, net.JoinHostPort(localNode, "22556"), 0, true))
	}

	// stay connected to DogeNet nodes.
	// for n := 0; n < DogeNetConnections; n++ {
	// 	gov.Add("core-crawler", NewDogeNet())
	// }

	// start connecting to Core Nodes.
	for n := 0; n < CoreNodeListeners; n++ {
		gov.Add(fmt.Sprintf("remote-%d", n), collector.New(&Map, "", 5*time.Minute, false))
	}

	// start the web server.
	gov.Add("web-api", web.New(&Map, "localhost", 8086))

	// run services until interrupted.
	gov.Start()
	gov.WaitForShutdown()
	fmt.Println("writing the netmap database…")
	Map.WriteGob(GobFilePath)
	fmt.Println("finished.")
}

// Map Saver service

type MapSaver struct {
	governor.ServiceCtx
}

func NewMapSaver() governor.Service {
	return &MapSaver{}
}

func (m *MapSaver) Run() {
	m0, q0 := Map.Stats()
	for {
		// save the map every 5 minutes, if modified
		if m.Sleep(5 * time.Minute) {
			return
		}
		Map.Trim()
		m1, q1 := Map.Stats()
		if m1 != m0 || q1 != q0 { // has changed?
			m.saveNetworkMap()
			m0 = m1
			q0 = q1
		}
	}
}

func (m *MapSaver) saveNetworkMap() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("error saving network map: %v", err)
		}
	}()
	Map.WriteGob(GobFilePath)
}
