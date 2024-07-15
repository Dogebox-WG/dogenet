package main

import (
	"log"
	"os"
	"strconv"

	"github.com/dogeorg/dogenet/internal/spec"
	dogenet "github.com/dogeorg/dogenet/pkg"
)

func main() {
	// required IP address of local node.
	// core node addresses are discovered via the local node.
	if len(os.Args) < 2 {
		log.Printf("usage: dogenet <local-core-node-ip> <port-offset>")
	}
	localNode := os.Args[1]
	webPort := 8085
	localPort := int(spec.DogeNetDefaultPort)
	remotePort := int(spec.DogeNetDefaultPort)
	if len(os.Args) > 2 {
		add, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Panicf("invalid port offset: %s", os.Args[2])
		}
		webPort += add
		localPort += add
	}
	dogenet.DogeNet(localNode, uint16(localPort), uint16(remotePort), uint16(webPort))
}
