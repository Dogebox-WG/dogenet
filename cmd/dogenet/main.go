package main

import (
	"os"

	dogenet "github.com/dogeorg/dogenet/pkg"
)

func main() {
	// optional IP address of local node.
	localNode := ""
	if len(os.Args) > 1 {
		localNode = os.Args[1]
	}
	dogenet.RunService(localNode)
}
