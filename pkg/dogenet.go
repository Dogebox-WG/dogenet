package dogenet

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/dogeorg/dogenet/internal/core/collector"
	"github.com/dogeorg/dogenet/internal/gossip"
	"github.com/dogeorg/dogenet/internal/governor"
	"github.com/dogeorg/dogenet/internal/spec"
	"github.com/dogeorg/dogenet/internal/store"
	"github.com/dogeorg/dogenet/internal/web"
)

const GobFilePath = "netmap.gob"

const DogeNetConnections = 4
const CoreNodeListeners = 1

func DogeNet(localNode string, localPort uint16, remotePort uint16, webPort uint16) {
	// load the previously saved state.
	db := store.New()
	err := db.LoadFrom(GobFilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Println("cannot read Gob file:", err)
		os.Exit(1)
	}

	gov := governor.New().CatchSignals().Restart(1 * time.Second)

	// periodically save the network map.
	gov.Add("map-saver", NewDBSaver(db))

	// start the gossip server
	if localPort == 0 {
		localPort = spec.DogeNetDefaultPort
	}
	gov.Add("gossip", gossip.New(spec.Address{Host: net.IPv4(0, 0, 0, 0), Port: localPort}, remotePort))

	// stay connected to local node if specified.
	if localNode != "" {
		addr := net.ParseIP(localNode)
		if addr == nil {
			log.Println("Invalid ip address on command line:", localNode)
			os.Exit(1)
		}
		gov.Add("local-node", collector.New(db, store.Address{Host: addr, Port: 22556}, 0, true))
	}

	// stay connected to DogeNet nodes.
	// for n := 0; n < DogeNetConnections; n++ {
	// 	gov.Add("core-crawler", NewDogeNet())
	// }

	// start connecting to Core Nodes.
	for n := 0; n < CoreNodeListeners; n++ {
		gov.Add(fmt.Sprintf("remote-%d", n), collector.New(db, store.Address{}, 5*time.Minute, false))
	}

	// start the web server.
	gov.Add("web-api", web.New(db, "localhost", int(webPort)))

	// run services until interrupted.
	gov.Start()
	gov.WaitForShutdown()
	fmt.Println("writing the netmap database…")
	db.Persist(GobFilePath)
	fmt.Println("finished.")
}

// Map Saver service

type DBSaver struct {
	governor.ServiceCtx
	store spec.Store
}

func NewDBSaver(store spec.Store) governor.Service {
	return &DBSaver{store: store}
}

func (m *DBSaver) Run() {
	m0, q0 := m.store.CoreStats()
	for {
		// save the map every 5 minutes, if modified
		if m.Sleep(5 * time.Minute) {
			return
		}
		m.store.TrimNodes()
		m1, q1 := m.store.CoreStats()
		if m1 != m0 || q1 != q0 { // has changed?
			m.saveNow()
			m0 = m1
			q0 = q1
		}
	}
}

func (m *DBSaver) saveNow() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("error saving network map: %v", err)
		}
	}()
	m.store.Persist(GobFilePath)
}
