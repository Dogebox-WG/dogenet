package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path"
	"strings"

	"code.dogecoin.org/gossip/dnet"

	"code.dogecoin.org/dogenet/internal/announce"
	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/dogenet/pkg/dogenet"
)

const WebAPIDefaultPort = 8085
const DogeNetDefaultPort = dnet.DogeNetDefaultPort
const DBFile = "dogenet.db"
const DefaultStorage = "./storage"

var HandlerDefaultBind = spec.BindTo{Network: "unix", Address: "/tmp/dogenet.sock"} // const

var stderr = log.New(os.Stderr, "", 0)

func main() {
	var allowLocal bool
	binds := []dnet.Address{}
	bindweb := []dnet.Address{}
	handlerBind := HandlerDefaultBind
	public := dnet.Address{}
	useReflector := false
	peers := []spec.NodeInfo{}
	dbfile := DBFile
	dir := DefaultStorage
	flag.Func("dir", "<path> - storage directory (default './storage')", func(arg string) error {
		ent, err := os.Stat(arg)
		if err != nil {
			stderr.Fatalf("--dir: %v", err)
		}
		if !ent.IsDir() {
			stderr.Fatalf("--dir: not a directory: %v", arg)
		}
		dir = arg
		return nil
	})
	flag.StringVar(&dbfile, "db", DBFile, "path to SQLite database (relative: in storage dir)")
	flag.BoolVar(&allowLocal, "local", false, "allow local 'public' addresses (for testing)")
	flag.Func("bind", "Bind gossip <ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		addr, err := parseIPPort(arg, "bind", DogeNetDefaultPort)
		if err != nil {
			return err
		}
		binds = append(binds, addr)
		return nil
	})
	flag.Func("web", "Bind web API <ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		addr, err := parseIPPort(arg, "web", WebAPIDefaultPort)
		if err != nil {
			return err
		}
		bindweb = append(bindweb, addr)
		return nil
	})
	flag.Func("handler", "Handler listen <ip>:<port> or /unix/path (use [<ip>]:<port> for IPv6)", func(arg string) error {
		bind, err := parseBindTo(arg, "handler")
		if err != nil {
			return err
		}
		handlerBind = bind
		return nil
	})
	flag.BoolVar(&useReflector, "reflector", false, fmt.Sprintf("Use reflector (%s) to obtain public (ISP) address", announce.ReflectorUrl))
	flag.Func("public", "Set public (ISP) gossip <ip>:<port> (use [<ip>]:<port> for IPv6)", func(arg string) error {
		// use DogeNetDefaultPort by default (rather than the --bind port)
		// this is typically correct even if bind-port is something different
		addr, err := parseIPPort(arg, "public", DogeNetDefaultPort)
		if err != nil {
			return err
		}
		public = addr
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
		addr, err := parseIPPort(arg, "peer", DogeNetDefaultPort)
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
			Port: DogeNetDefaultPort,
		})
	}
	if len(bindweb) < 1 {
		bindweb = append(bindweb, dnet.Address{
			Host: net.IP([]byte{0, 0, 0, 0}),
			Port: WebAPIDefaultPort,
		})
	}
	if public.IsValid() {
		if !allowLocal && (!public.Host.IsGlobalUnicast() || public.Host.IsPrivate()) {
			log.Printf("bad --public address: cannot be a private or multicast address")
			os.Exit(1)
		}
		useReflector = false // valid --public IP overrides --reflector
	} else if !useReflector {
		log.Printf("node public address must be specified via --public or --reflector")
		os.Exit(1)
	}

	// get the private key from the KEY env-var
	nodeKey := keysFromEnv()
	log.Printf("Node PubKey is: %v", hex.EncodeToString(nodeKey.Pub[:]))

	service := dogenet.DogeNetService{
		NodeKey:      nodeKey,
		Binds:        binds,
		BindWeb:      bindweb,
		Dir:          dir,
		DBFile:       dbfile,
		HandlerBind:  handlerBind,
		AllowLocal:   allowLocal,
		Public:       public,
		UseReflector: useReflector,
	}

	err := service.Start()
	if err != nil {
		log.Fatal(err)
	}

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

func parseBindTo(arg string, name string) (spec.BindTo, error) {
	if strings.HasPrefix(arg, "/") {
		// unix socket path.
		ent, err := os.Stat(arg)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// not found: ensure parent dir exists.
				dir := path.Dir(arg)
				ent, err := os.Stat(dir)
				if err != nil || !ent.IsDir() {
					return spec.BindTo{}, fmt.Errorf("bad --%v: directory not found: %v", name, dir)
				}
				// valid binding.
				return spec.BindTo{Network: "unix", Address: arg}, nil
			}
			return spec.BindTo{}, fmt.Errorf("bad --%v: %v", name, err)
		}
		if !ent.IsDir() {
			// exists, not a directory.
			err = os.Remove(arg)
			if err != nil {
				return spec.BindTo{}, fmt.Errorf("bad --%v: cannot remove existing file: %v", name, arg)
			}
			// valid binding.
			return spec.BindTo{Network: "unix", Address: arg}, nil
		} else {
			return spec.BindTo{}, fmt.Errorf("bad --%v: path is a directory: %v", name, arg)
		}
	} else {
		addr, err := parseIPPort(arg, name, 0)
		if err != nil {
			return spec.BindTo{}, fmt.Errorf("bad --%v: %v", name, err)
		}
		if addr.Port == 0 {
			return spec.BindTo{}, fmt.Errorf("bad --%v: must specify a port", name)
		}
		// valid binding.
		return spec.BindTo{Network: "tcp", Address: addr.String()}, nil
	}
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
