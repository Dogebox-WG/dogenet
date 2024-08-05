package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"code.dogecoin.org/gossip/dnet"

	"code.dogecoin.org/governor"

	"code.dogecoin.org/dogenet/internal/core/collector"
	"code.dogecoin.org/dogenet/internal/netsvc"
	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/dogenet/internal/store"
	"code.dogecoin.org/dogenet/internal/web"
)

const WebAPIDefaultPort = 8085
const CoreNodeDefaultPort = 22556
const StoreFilename = "storage/dogenet.db"

func main() {
	var crawl int
	binds := []dnet.Address{}
	bindweb := []dnet.Address{}
	public := dnet.Address{}
	core := dnet.Address{}
	peers := []spec.NodeInfo{}
	dbfile := StoreFilename

	flag.IntVar(&crawl, "crawl", 0, "number of core node crawlers")
	flag.StringVar(&dbfile, "db", StoreFilename, "path to SQLite database")
	flag.Func("bind", "<ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		addr, err := parseIPPort(arg, "bind", dnet.DogeNetDefaultPort)
		if err != nil {
			return err
		}
		binds = append(binds, addr)
		return nil
	})
	flag.Func("web", "<ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		addr, err := parseIPPort(arg, "web", WebAPIDefaultPort)
		if err != nil {
			return err
		}
		bindweb = append(bindweb, addr)
		return nil
	})
	flag.Func("public", "<ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		addr, err := parseIPPort(arg, "public", dnet.DogeNetDefaultPort)
		if err != nil {
			return err
		}
		public = addr
		return nil
	})
	flag.Func("core", "<ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		addr, err := parseIPPort(arg, "core", CoreNodeDefaultPort)
		if err != nil {
			return err
		}
		core = addr
		return nil
	})
	flag.Func("peer", "<pubkey>:<ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("bad --peer: expecting ':' in argument: %v", arg)
		}
		pub, err := hex.DecodeString(parts[0])
		if err != nil || len(pub) != 32 {
			return fmt.Errorf("bad --peer: invalid hex pubkey: %v", parts[0])
		}
		addr, err := parseIPPort(arg, "peer", dnet.DogeNetDefaultPort)
		if err != nil {
			return err
		}
		peers = append(peers, spec.NodeInfo{
			PubKey: pub,
			Addr:   addr,
		})
		return nil
	})
	flag.Parse()
	if flag.NArg() > 0 {
		cmd := flag.Arg(0)
		switch cmd {
		case "genkey":
			nodeKey, err := dnet.GenerateKeyPair()
			if err != nil {
				panic(fmt.Sprintf("cannot generate node keypair: %v", err))
			}
			fmt.Printf("%v", hex.EncodeToString(nodeKey.Priv))
			os.Exit(0)
		default:
			log.Printf("Unexpected argument: %v", cmd)
			os.Exit(1)
		}
	}
	if len(binds) < 1 {
		binds = append(binds, dnet.Address{
			Host: net.IP([]byte{0, 0, 0, 0}),
			Port: dnet.DogeNetDefaultPort,
		})
	}
	if len(bindweb) < 1 {
		bindweb = append(bindweb, dnet.Address{
			Host: net.IP([]byte{0, 0, 0, 0}),
			Port: WebAPIDefaultPort,
		})
	}

	// get the private key from the KEY env-var
	nodeKey := keyPairFromEnv()
	log.Printf("Node PubKey is: %v", hex.EncodeToString(nodeKey.Pub))

	// load the previously saved state.
	db, err := store.NewSQLiteStore(dbfile, context.Background())
	if err != nil {
		log.Printf("Error opening database: %v [%s]\n", err, dbfile)
		os.Exit(1)
	}

	gov := governor.New().CatchSignals().Restart(1 * time.Second)

	// start the gossip server
	netSvc := netsvc.New(binds, public, db, nodeKey)
	gov.Add("gossip", netSvc)

	// stay connected to local node if specified.
	if core.IsValid() {
		gov.Add("local-node", collector.New(db, core, 0, true))
	}

	// start crawling Core Nodes.
	for n := 0; n < crawl; n++ {
		gov.Add(fmt.Sprintf("crawler-%d", n), collector.New(db, store.Address{}, 5*time.Minute, false))
	}

	// start the web server.
	for _, bind := range bindweb {
		gov.Add("web-api", web.New(bind, db, netSvc))
	}

	// run services until interrupted.
	gov.Start()
	gov.WaitForShutdown()
	fmt.Println("finished.")
}

// Parse an IPv4 or IPv6 address with optional port.
func parseIPPort(arg string, name string, defaultPort uint16) (dnet.Address, error) {
	// net.SplitHostPort doesn't return a specific error code,
	// so we need to detect if the port it present manually.
	colon := strings.LastIndex(arg, ":")
	bracket := strings.LastIndex(arg, "]")
	if colon == -1 || (arg[0] == '[' && bracket != -1 && colon < bracket) {
		ip := net.ParseIP(arg)
		if ip == nil {
			return dnet.Address{}, fmt.Errorf("bad --%v: invalid IP address: %v (use [<ip>]:port for IPv6)", name, arg)
		}
		return dnet.Address{
			Host: ip,
			Port: defaultPort,
		}, nil
	}
	res, err := dnet.ParseAddress(arg)
	if err != nil {
		return dnet.Address{}, fmt.Errorf("bad --%v: invalid IP address: %v (use [<ip>]:port for IPv6)", name, arg)
	}
	return res, nil
}

func keyPairFromEnv() dnet.KeyPair {
	// get the private key from the KEY env-var
	keyHex := os.Getenv("KEY")
	os.Setenv("KEY", "") // don't leave the key in the environment
	privPub, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Printf("Invalid KEY hex in env-var: %v", err)
		os.Exit(3)
	}
	if len(privPub) != 64 {
		log.Printf("Invalid KEY hex in env-var: must be 64 bytes (see `dogenet genkey`)")
		os.Exit(3)
	}
	return dnet.KeyPairFromPrivKey(privPub)
}
