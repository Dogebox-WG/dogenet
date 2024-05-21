package main

import (
	"log"
	"os"

	dogenet "github.com/dogeorg/dogenet/pkg"
)

func main() {
	// required IP address of local node.
	// core node addresses are discovered via the local node.
	if len(os.Args) < 2 {
		log.Printf("usage: dogenet <local-core-node-ip>")
	}
	localNode := os.Args[1]
	dogenet.DogeNet(localNode)
}
