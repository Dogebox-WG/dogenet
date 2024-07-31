package store

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"

	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/gossip/node"
	"github.com/mattn/go-sqlite3"
)

type NodeID = spec.NodeID
type Address = spec.Address

// SELECT * FROM table WHERE id IN (SELECT id FROM table ORDER BY RANDOM() LIMIT 10)

type SQLiteStore struct {
	db *sql.DB
}

var _ spec.Store = SQLiteStore{}

// The common read-only parts of sql.DB and sql.Tx interfaces
type Queryable interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// WITHOUT ROWID: SQLite version 3.8.2 (2013-12-06) or later

const SQL_SCHEMA string = `
CREATE TABLE IF NOT EXISTS config (
	dayc INTEGER NOT NULL,
	last DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS core (
	address BLOB NOT NULL PRIMARY KEY,
	time DATETIME NOT NULL,
	services INTEGER NOT NULL,
	isnew BOOLEAN NOT NULL,
	dayc INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS core_time_i ON core (time);
CREATE INDEX IF NOT EXISTS core_isnew_i ON core (isnew);
CREATE TABLE IF NOT EXISTS node (
	key BLOB NOT NULL PRIMARY KEY,
	address BLOB NOT NULL,
	time DATETIME NOT NULL,
	msg BLOB NOT NULL,
	dayc INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS node_time_i ON node (time);
CREATE INDEX IF NOT EXISTS node_address_i ON node (address);
CREATE INDEX IF NOT EXISTS node_isnew_i ON node (isnew);
CREATE TABLE IF NOT EXISTS chan (
	node INTEGER NOT NULL,
	chan INTEGER NOT NULL,
	PRIMARY KEY (node, chan)
) WITHOUT ROWID;
`

// NewSQLiteStore returns a giga.PaymentsStore implementor that uses sqlite
func NewSQLiteStore(fileName string) (spec.Store, error) {
	backend := "sqlite3"
	db, err := sql.Open(backend, fileName)
	store := SQLiteStore{db: db}
	if err != nil {
		return SQLiteStore{}, store.dbErr(err, "opening database")
	}
	setup_sql := SQL_SCHEMA
	if backend == "sqlite3" {
		// limit concurrent access until we figure out a way to start transactions
		// with the BEGIN CONCURRENT statement in Go.
		db.SetMaxOpenConns(1)
	}
	// init tables / indexes
	_, err = db.Exec(setup_sql)
	if err != nil {
		return SQLiteStore{}, store.dbErr(err, "creating database schema")
	}
	return store, nil
}

func (s SQLiteStore) Close() {
	s.db.Close()
}

func (s SQLiteStore) dbErr(err error, where string) error {
	if sqErr, isSq := err.(sqlite3.Error); isSq {
		if sqErr.Code == sqlite3.ErrConstraint {
			// MUST detect 'AlreadyExists' to fulfil the API contract!
			// Constraint violation, e.g. a duplicate key.
			return WrapErr(AlreadyExists, "SQLiteStore: already-exists", err)
		}
		if sqErr.Code == sqlite3.ErrBusy || sqErr.Code == sqlite3.ErrLocked {
			// SQLite has a single-writer policy, even in WAL (write-ahead) mode.
			// SQLite will return BUSY if the database is locked by another connection.
			// We treat this as a transient database conflict, and the caller should retry.
			return WrapErr(DBConflict, "SQLiteStore: db-conflict", err)
		}
	}
	return WrapErr(DBProblem, fmt.Sprintf("SQLiteStore: db-problem: %s", where), err)
}

// STORE INTERFACE

func (s SQLiteStore) NodeKey() (pub spec.PubKey, priv spec.PrivKey) {
	return []byte{}, []byte{}
}

func (s SQLiteStore) CoreStats() (mapSize int, newNodes int) {
	row := s.db.QueryRow("SELECT COUNT(core) AS num FROM core UNION SELECT COUNT(core) AS isnew FROM node WHERE isnew=TRUE")
	var num, isnew int
	err := row.Scan(&num, &isnew)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[Store] CoreStats: %v", err)
		}
		return 0, 0
	}
	return num, isnew
}

func (s SQLiteStore) NetStats() (mapSize int, newNodes int) {
	row := s.db.QueryRow("SELECT COUNT(node) AS num FROM node UNION SELECT COUNT(node) AS isnew FROM node WHERE isnew=TRUE")
	var num, isnew int
	err := row.Scan(&num, &isnew)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[Store] NodeStats: %v", err)
		}
		return 0, 0
	}
	return num, isnew
}

func (s SQLiteStore) coreNodeList() (res []spec.CoreNode) {
	rows, err := s.db.Query("SELECT address,time,services FROM core")
	if err != nil {
		log.Printf("[Store] coreNodeList: query: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var addr []byte
		var time int64
		var services uint64
		err := rows.Scan(&addr, &time, &services)
		if err != nil {
			log.Printf("[Store] coreNodeList: scanning row: %v", err)
			continue
		}
		s_adr, err := dnet.AddressFromBytes(addr)
		if err != nil {
			log.Printf("[Store] bad node address: %v", err)
			continue
		}
		res = append(res, spec.CoreNode{
			Address:  s_adr.String(),
			Time:     time,
			Services: services,
		})
	}
	if err = rows.Err(); err != nil { // docs say this check is required!
		log.Printf("[Store] query: %v", err)
	}
	return
}

func (s SQLiteStore) netNodeList() (res []spec.NetNode) {
	rows, err := s.db.Query("SELECT msg,time FROM node")
	if err != nil {
		log.Printf("[Store] netNodeList: query: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var msg []byte
		var unixTime int64
		err := rows.Scan(&msg, &unixTime)
		if err != nil {
			log.Printf("[Store] netNodeList: scanning row: %v", err)
			continue
		}
		view := dnet.MsgView(msg)
		pubkey := hex.EncodeToString(view.PubKey())
		amsg := node.DecodeAddrMsg(view.Payload())
		ident := hex.EncodeToString(amsg.Owner)
		addr := Address{
			Host: net.IP(amsg.Address),
			Port: amsg.Port,
		}
		channels := make([]string, 0, len(amsg.Channels))
		for _, c := range amsg.Channels {
			channels = append(channels, c.String())
		}
		res = append(res, spec.NetNode{
			PubKey:   pubkey,
			Address:  addr.String(),
			Time:     unixTime,
			Channels: channels,
			Identity: ident,
		})
	}
	if err = rows.Err(); err != nil { // docs say this check is required!
		log.Printf("[Store] query: %v", err)
	}
	return
}

func (s SQLiteStore) NodeList() (res spec.NodeListRes) {
	res.Core = s.coreNodeList()
	res.Net = s.netNodeList()
	return
}

func (s SQLiteStore) TrimNodes() {
}

func (s SQLiteStore) AddCoreNode(address Address, time int64, services uint64) {
}

func (s SQLiteStore) UpdateCoreTime(address Address) {
}

func (s SQLiteStore) ChooseCoreNode() Address {
	return Address{}
}

func (s SQLiteStore) SampleCoreNodes() []Address {
	return []Address{}
}

func (s SQLiteStore) AddNetNode(pubkey spec.PubKey, address Address, time int64, channels []dnet.Tag4CC, msg []byte) {
}

func (s SQLiteStore) UpdateNetTime(key spec.PubKey) {
}

func (s SQLiteStore) ChooseNetNode() spec.NodeInfo {
	return spec.NodeInfo{}
}

func (s SQLiteStore) SampleNetNodes() []spec.NodeInfo {
	return []spec.NodeInfo{}
}

func (s SQLiteStore) SampleNodesByChannel(channels []dnet.Tag4CC, exclude []spec.PubKey) []spec.NodeInfo {
	return []spec.NodeInfo{}
}

func (s SQLiteStore) SampleNodesByIP(ipaddr net.IP, exclude []spec.PubKey) []spec.NodeInfo {
	return []spec.NodeInfo{}
}
