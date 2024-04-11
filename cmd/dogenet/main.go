package main

import (
	"fmt"
	"net"
	"os"

	dogenet "github.com/dogeorg/dogenet/pkg"
)

func main() {

	// Check if an IP address is provided as a command line argument
	if len(os.Args) > 1 {
		// Test the Dogecoin node
		dogenet.TestNode(os.Args[1])
	} else {
		// Query all Dogecoin network
		seedNodes := []string{"seed.multidoge.org"}
		for _, node := range seedNodes {
			ips, err := net.LookupIP(node)
			if err != nil {
				fmt.Println("Error resolving DNS:", err)
				continue
			}
			for _, ip := range ips {
				dogenet.TestNode(ip.String())
			}
		}
	}

}
