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

	"code.dogecoin.org/dogenet/internal/announce"
	"code.dogecoin.org/dogenet/internal/core/collector"
	"code.dogecoin.org/dogenet/internal/geoip"
	"code.dogecoin.org/dogenet/internal/netsvc"
	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/dogenet/internal/store"
	"code.dogecoin.org/dogenet/internal/web"
)

const WebAPIDefaultPort = 8085
const CoreNodeDefaultPort = 22556
const StoreFilename = "storage/dogenet.db"
const GeoIPFile = "storage/dbip-city-ipv4-num.csv"

func main() {
	var crawl int
	var allowLocal bool
	binds := []dnet.Address{}
	bindweb := []dnet.Address{}
	public := dnet.Address{}
	core := dnet.Address{}
	peers := []spec.NodeInfo{}
	dbfile := StoreFilename

	flag.IntVar(&crawl, "crawl", 0, "number of core node crawlers")
	flag.StringVar(&dbfile, "db", StoreFilename, "path to SQLite database")
	flag.BoolVar(&allowLocal, "local", false, "allow local 'public' addresses (for testing)")
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
		// use DogeNetDefaultPort by default (rather than the --bind port)
		// this is typically correct even if bind-port is something different
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
			PubKey: ([32]byte)(pub),
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
			priv := hex.EncodeToString(nodeKey.Priv[:])
			pub := hex.EncodeToString(nodeKey.Pub[:])
			if flag.NArg() > 1 {
				to_priv := flag.Arg(1)
				os.WriteFile(to_priv, []byte(priv), 0666)
				if flag.NArg() > 2 {
					to_pub := flag.Arg(2)
					os.WriteFile(to_pub, []byte(pub), 0666)
				}
			} else {
				fmt.Printf("priv: %v\n", priv)
				fmt.Printf("pub: %v\n", pub)
			}
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
	if !public.IsValid() {
		log.Printf("node public address must be specified via --public")
		os.Exit(1)
	}
	if !allowLocal && (!public.Host.IsGlobalUnicast() || public.Host.IsPrivate()) {
		log.Printf("bad --public address: cannot be a private or multicast address")
		os.Exit(1)
	}

	// get the private key from the KEY env-var
	nodeKey := keysFromEnv()
	log.Printf("Node PubKey is: %v", hex.EncodeToString(nodeKey.Pub[:]))

	// load the previously saved state.
	db, err := store.NewSQLiteStore(dbfile, context.Background())
	if err != nil {
		log.Printf("Error opening database: %v [%s]\n", err, dbfile)
		os.Exit(1)
	}

	gov := governor.New().CatchSignals().Restart(1 * time.Second)

	// start the gossip server
	changes := make(chan any, 10)
	netSvc := netsvc.New(binds, nodeKey, db, allowLocal, changes)
	gov.Add("gossip", netSvc)

	// start the announcement service
	gov.Add("announce", announce.New(public, nodeKey, db, netSvc, changes))

	// stay connected to local node if specified.
	if core.IsValid() {
		gov.Add("local-node", collector.New(db, core, 60*time.Second, true))
	}

	// start crawling Core Nodes.
	for n := 0; n < crawl; n++ {
		gov.Add(fmt.Sprintf("crawler-%d", n), collector.New(db, store.Address{}, 5*time.Minute, false))
	}

	// load the geoIP database
	// https://github.com/sapics/ip-location-db/tree/main/dbip-city/dbip-city-ipv4-num.csv.gz
	log.Printf("loading GeoIP database: %v", GeoIPFile)
	geoIP, err := geoip.NewGeoIPDatabase(GeoIPFile)
	if err != nil {
		log.Printf("Error reading GeoIP database: %v [%s]\n", err, GeoIPFile)
		os.Exit(1)
	}

	// start the web server.
	for _, bind := range bindweb {
		gov.Add("web-api", web.New(bind, db, netSvc, geoIP))
	}

	// start the store trimmer
	gov.Add("store", store.NewStoreTrimmer(db))

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

func keysFromEnv() dnet.KeyPair {
	// get the private key from the KEY env-var
	nodeHex := os.Getenv("KEY")
	os.Setenv("KEY", "") // don't leave the key in the environment
	if nodeHex == "" {
		log.Printf("Missing KEY env-var: node public-private keypair (32 bytes; see `dogenet genkey`)")
		os.Exit(3)
	}
	nodeKey, err := hex.DecodeString(nodeHex)
	if err != nil {
		log.Printf("Invalid KEY hex in env-var: %v", err)
		os.Exit(3)
	}
	if len(nodeKey) != 32 {
		log.Printf("Invalid KEY hex in env-var: must be 32 bytes")
		os.Exit(3)
	}
	return dnet.KeyPairFromPrivKey((*[32]byte)(nodeKey))
}
