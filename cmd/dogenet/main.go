package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"rad/gossip/dnet"
	"strconv"
	"time"

	"rad/governor"

	"github.com/dogeorg/dogenet/internal/core/collector"
	"github.com/dogeorg/dogenet/internal/netsvc"
	"github.com/dogeorg/dogenet/internal/spec"
	"github.com/dogeorg/dogenet/internal/store"
	"github.com/dogeorg/dogenet/internal/web"
)

const DogeNetConnections = 4
const CoreNodeListeners = 1

func DogeNetMain(localNode string, localPort uint16, remotePort uint16, webPort uint16) {
	// load the previously saved state.
	db := store.LoadStore()
	gov := governor.New().CatchSignals().Restart(1 * time.Second)

	// periodically save the network map.
	gov.Add("map-saver", store.NewStoreSaver(db))

	// start the gossip server
	if localPort == 0 {
		localPort = dnet.DogeNetDefaultPort
	}
	gov.Add("gossip", netsvc.New(spec.Address{Host: net.IPv4(0, 0, 0, 0), Port: localPort}, remotePort, db))

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
	// 	gov.Add("", NewDogeNet())
	// }

	// start connecting to Core Nodes.
	// for n := 0; n < CoreNodeListeners; n++ {
	// 	gov.Add(fmt.Sprintf("remote-%d", n), collector.New(db, store.Address{}, 5*time.Minute, false))
	// }

	// start the web server.
	gov.Add("web-api", web.New(db, "localhost", int(webPort)))

	// run services until interrupted.
	gov.Start()
	gov.WaitForShutdown()
	fmt.Println("writing the netmap database…")
	db.Persist(store.GobFilePath)
	fmt.Println("finished.")
}

func main() {
	// required IP address of local node.
	// core node addresses are discovered via the local node.
	if len(os.Args) < 2 {
		log.Printf("usage: dogenet <local-core-node-ip> <port-offset>")
	}
	localNode := os.Args[1]
	webPort := 8085
	localPort := int(dnet.DogeNetDefaultPort)
	remotePort := int(dnet.DogeNetDefaultPort)
	if len(os.Args) > 2 {
		add, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Panicf("invalid port offset: %s", os.Args[2])
		}
		webPort += add
		localPort += add
	}
	DogeNetMain(localNode, uint16(localPort), uint16(remotePort), uint16(webPort))
}
