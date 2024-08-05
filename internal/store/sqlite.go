package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

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

type SQLiteStoreCtx struct {
	db  *sql.DB
	ctx context.Context
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
CREATE TABLE IF NOT EXISTS announce (
	payload BLOB NOT NULL,
	sig BLOB NOT NULL,
	time DATETIME NOT NULL
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
	owner BLOB NOT NULL,
	payload BLOB NOT NULL,
	sig BLOB NOT NULL,
	dayc INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS node_time_i ON node (time);
CREATE INDEX IF NOT EXISTS node_address_i ON node (address);
CREATE TABLE IF NOT EXISTS chan (
	node INTEGER NOT NULL,
	chan INTEGER NOT NULL,
	PRIMARY KEY (node, chan)
) WITHOUT ROWID;
`

// NewSQLiteStore returns a giga.PaymentsStore implementor that uses sqlite
func NewSQLiteStore(fileName string, ctx context.Context) (spec.Store, error) {
	backend := "sqlite3"
	db, err := sql.Open(backend, fileName)
	store := SQLiteStore{db: db}
	if err != nil {
		return SQLiteStore{}, dbErr(err, "opening database")
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
		return SQLiteStore{}, dbErr(err, "creating database schema")
	}
	// init config table
	sctx := SQLiteStoreCtx{db: store.db, ctx: ctx}
	err = sctx.doTxn("init config", func(tx *sql.Tx) error {
		config := tx.QueryRow("SELECT dayc,last FROM config LIMIT 1")
		var dayc int
		var last time.Time
		err = config.Scan(&dayc, &last)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				_, err = tx.Exec("INSERT INTO config (dayc,last) VALUES (1,CURRENT_TIMESTAMP)")
			}
			return err
		}
		return nil
	})
	return store, err
}

func (s SQLiteStore) Close() {
	s.db.Close()
}

func (s SQLiteStore) WithCtx(ctx context.Context) spec.StoreCtx {
	return &SQLiteStoreCtx{
		db:  s.db,
		ctx: ctx,
	}
}

func IsConflict(err error) bool {
	if sqErr, isSq := err.(sqlite3.Error); isSq {
		if sqErr.Code == sqlite3.ErrBusy || sqErr.Code == sqlite3.ErrLocked {
			return true
		}
	}
	return false
}

func (s SQLiteStoreCtx) doTxn(name string, work func(tx *sql.Tx) error) error {
	db := s.db
	limit := 120
	for {
		tx, err := db.Begin()
		if err != nil {
			if IsConflict(err) {
				s.Sleep(250 * time.Millisecond)
				limit--
				if limit != 0 {
					continue
				}
			}
			return fmt.Errorf("[Store] cannot begin transaction: %v", err)
		}
		defer tx.Rollback()
		err = work(tx)
		if err != nil {
			if IsConflict(err) {
				s.Sleep(250 * time.Millisecond)
				limit--
				if limit != 0 {
					continue
				}
			}
			return fmt.Errorf("[Store] %v: %v", name, err)
		}
		err = tx.Commit()
		if err != nil {
			if IsConflict(err) {
				s.Sleep(250 * time.Millisecond)
				limit--
				if limit != 0 {
					continue
				}
			}
			return fmt.Errorf("[Store] cannot commit %v: %v", name, err)
		}
		return nil
	}
}

func (s SQLiteStoreCtx) Sleep(dur time.Duration) {
	select {
	case <-s.ctx.Done():
	case <-time.After(dur):
	}
}

func dbErr(err error, where string) error {
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

func (s SQLiteStoreCtx) NodeKey() (pub spec.PubKey, priv spec.PrivKey) {
	return []byte{}, []byte{}
}

func (s SQLiteStoreCtx) CoreStats() (mapSize int, newNodes int) {
	row := s.db.QueryRow("WITH t AS (SELECT COUNT(address) AS num, 1 AS rn FROM core), u AS (SELECT COUNT(address) AS isnew, 1 AS rn FROM core WHERE isnew=TRUE) SELECT t.num, u.isnew FROM t INNER JOIN u ON t.rn=u.rn")
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

func (s SQLiteStoreCtx) NetStats() (mapSize int, newNodes int) {
	row := s.db.QueryRow("SELECT COUNT(key) AS num FROM node")
	var num, isnew int
	err := row.Scan(&num)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[Store] NodeStats: %v", err)
		}
		return 0, 0
	}
	return num, isnew
}

func (s SQLiteStoreCtx) coreNodeList() (res []spec.CoreNode) {
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

func (s SQLiteStoreCtx) netNodeList() (res []spec.NetNode) {
	// use payload because it contains all the channels
	rows, err := s.db.Query("SELECT key,payload,time FROM node")
	if err != nil {
		log.Printf("[Store] netNodeList: query: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pubkey []byte
		var payload []byte
		var unixTime int64
		err := rows.Scan(&pubkey, &payload, &unixTime)
		if err != nil {
			log.Printf("[Store] netNodeList: scanning row: %v", err)
			continue
		}
		amsg := node.DecodeAddrMsg(payload)
		addr := Address{
			Host: net.IP(amsg.Address),
			Port: amsg.Port,
		}
		channels := make([]string, 0, len(amsg.Channels))
		for _, c := range amsg.Channels {
			channels = append(channels, c.String())
		}
		res = append(res, spec.NetNode{
			PubKey:   hex.EncodeToString(pubkey),
			Address:  addr.String(),
			Time:     unixTime,
			Channels: channels,
			Identity: hex.EncodeToString(amsg.Owner),
		})
	}
	if err = rows.Err(); err != nil { // docs say this check is required!
		log.Printf("[Store] query: %v", err)
	}
	return
}

func (s SQLiteStoreCtx) NodeList() (res spec.NodeListRes) {
	res.Core = s.coreNodeList()
	res.Net = s.netNodeList()
	return
}

func (s SQLiteStoreCtx) TrimNodes() {
}

func (s SQLiteStoreCtx) AddCoreNode(address Address, time int64, services uint64) {
}

func (s SQLiteStoreCtx) UpdateCoreTime(address Address) {
}

func (s SQLiteStoreCtx) ChooseCoreNode() Address {
	return Address{}
}

func (s SQLiteStoreCtx) SampleCoreNodes() []Address {
	return []Address{}
}

func (s SQLiteStoreCtx) GetAnnounce() (payload []byte, sig []byte, time int64, err error) {
	row := s.db.QueryRow("SELECT payload,sig,time FROM announce LIMIT 1")
	e := row.Scan(&payload, &sig, &time)
	if e != nil {
		if !errors.Is(e, sql.ErrNoRows) {
			err = fmt.Errorf("query: %v", e)
		}
	}
	return
}

func (s SQLiteStoreCtx) SetAnnounce(payload []byte, sig []byte, time int64) error {
	return s.doTxn("SetAnnounce", func(tx *sql.Tx) error {
		res, err := s.db.Exec("UPDATE announce SET payload=?,sig=?,time=?", payload, sig, time)
		if err != nil {
			return err
		}
		num, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if num == 0 {
			_, err = s.db.Exec("INSERT INTO announce (payload,sig,time) VALUES (?,?,?)", payload, sig, time)
		}
		return err
	})
}

// const add_netnode_psql = "INSERT INTO node (key, address, time, owner, payload, sig, dayc) VALUES (?1,?2,?3,?4,?5,?6,30+(SELECT dayc FROM config LIMIT 1)) ON CONFLICT ON CONSTRAINT node_key DO UPDATE SET address=?2, time=?3, owner=?4, payload=?5, sig=?6, dayc=30+(SELECT dayc FROM config LIMIT 1)"
// const add_netnode_sqlite = "INSERT INTO node (key, address, time, owner, payload, sig, dayc) VALUES (?1,?2,?3,?4,?5,?6,30+(SELECT dayc FROM config LIMIT 1)) ON CONFLICT REPLACE RETURNING oid"

func (s SQLiteStoreCtx) AddNetNode(key spec.PubKey, address Address, time int64, owner spec.PubKey, channels []dnet.Tag4CC, payload []byte, sig []byte) (changed bool, err error) {
	err = s.doTxn("AddNetNode", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT oid,payload FROM node WHERE key=? LIMIT 1", key)
		var oid int64
		var stored []byte
		e := row.Scan(&oid, &stored)
		if e != nil {
			// no rows found, or an error.
			if !errors.Is(e, sql.ErrNoRows) {
				return fmt.Errorf("query: %v", e)
			}
			// no rows found: must insert the node.
			res, e := tx.Exec("INSERT INTO node (key, address, time, owner, payload, sig, dayc) VALUES (?1,?2,?3,?4,?5,?6,30+(SELECT dayc FROM config LIMIT 1))",
				key, address.ToBytes(), time, owner, payload, sig)
			if e != nil {
				return fmt.Errorf("insert: %v", e)
			}
			oid, e = res.LastInsertId()
			if e != nil {
				return fmt.Errorf("lastid: %v", e)
			}
		} else {
			if bytes.Equal(stored, payload) {
				return nil // existing row has the same payload: no change.
			}
			// payload is different: must update the row.
			_, e := tx.Exec("UPDATE node SET address=?, time=?, owner=?, payload=?, sig=?, dayc=30+(SELECT dayc FROM config LIMIT 1) WHERE key=?",
				address.ToBytes(), time, owner, payload, sig, key)
			if e != nil {
				return fmt.Errorf("update: %v", e)
			}
		}
		_, e = tx.Exec("DELETE FROM chan WHERE node=?", oid)
		if e != nil {
			return fmt.Errorf("delete channels: %v", e)
		}
		ins, e := tx.Prepare("INSERT INTO chan (node,chan) VALUES (?,?)")
		if e != nil {
			return fmt.Errorf("prepare: %v", e)
		}
		for _, channel := range channels {
			_, e = ins.Exec(oid, channel.String())
			if e != nil {
				return fmt.Errorf("insert channel: %v", e)
			}
		}
		changed = true
		return nil
	})
	return
}

func (s SQLiteStoreCtx) UpdateNetTime(key spec.PubKey) {
}

func (s SQLiteStoreCtx) ChooseNetNode() spec.NodeInfo {
	return spec.NodeInfo{}
}

func (s SQLiteStoreCtx) SampleNetNodes() []spec.NodeInfo {
	return []spec.NodeInfo{}
}

func (s SQLiteStoreCtx) SampleNodesByChannel(channels []dnet.Tag4CC, exclude []spec.PubKey) []spec.NodeInfo {
	return []spec.NodeInfo{}
}

func (s SQLiteStoreCtx) SampleNodesByIP(ipaddr net.IP, exclude []spec.PubKey) []spec.NodeInfo {
	return []spec.NodeInfo{}
}
