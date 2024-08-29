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

const SecondsPerDay = 24 * 60 * 60

// SELECT * FROM table WHERE id IN (SELECT id FROM table ORDER BY RANDOM() LIMIT 10)

type SQLiteStore struct {
	db *sql.DB
}

type SQLiteStoreCtx struct {
	_db *sql.DB
	ctx context.Context
}

var _ spec.Store = &SQLiteStore{}

// The common read-only parts of sql.DB and sql.Tx interfaces
type Queryable interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// WITHOUT ROWID: SQLite version 3.8.2 (2013-12-06) or later

const SQL_SCHEMA string = `
CREATE TABLE IF NOT EXISTS config (
	dayc INTEGER NOT NULL,
	last INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS announce (
	payload BLOB NOT NULL,
	sig BLOB NOT NULL,
	time INTEGER NOT NULL
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

// NewSQLiteStore returns a spec.Store implementation that uses SQLite
func NewSQLiteStore(fileName string, ctx context.Context) (spec.Store, error) {
	backend := "sqlite3"
	db, err := sql.Open(backend, fileName)
	store := &SQLiteStore{db: db}
	if err != nil {
		return store, dbErr(err, "opening database")
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
		return store, dbErr(err, "creating database schema")
	}
	// init config table
	sctx := SQLiteStoreCtx{_db: store.db, ctx: ctx}
	err = sctx.doTxn("init config", func(tx *sql.Tx) error {
		config := tx.QueryRow("SELECT dayc,last FROM config LIMIT 1")
		var dayc int64
		var last int64
		err = config.Scan(&dayc, &last)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				_, err = tx.Exec("INSERT INTO config (dayc,last) VALUES (1,?)", unixDayStamp())
			}
			return err
		}
		return nil
	})
	return store, err
}

func (s *SQLiteStore) Close() {
	s.db.Close()
}

func (s *SQLiteStore) WithCtx(ctx context.Context) spec.StoreCtx {
	return &SQLiteStoreCtx{
		_db: s.db,
		ctx: ctx,
	}
}

// The number of whole days since the unix epoch.
func unixDayStamp() int64 {
	return time.Now().Unix() / SecondsPerDay
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
	db := s._db
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

func (s SQLiteStoreCtx) CoreStats() (mapSize int, newNodes int, err error) {
	err = s.doTxn("CoreStats", func(tx *sql.Tx) error {
		row := tx.QueryRow("WITH t AS (SELECT COUNT(address) AS num, 1 AS rn FROM core), u AS (SELECT COUNT(address) AS isnew, 1 AS rn FROM core WHERE isnew=TRUE) SELECT t.num, u.isnew FROM t INNER JOIN u ON t.rn=u.rn")
		err := row.Scan(&mapSize, &newNodes)
		if err != nil {
			if err != sql.ErrNoRows {
				log.Printf("[Store] CoreStats: %v", err)
			}
			return nil
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) NetStats() (mapSize int, err error) {
	err = s.doTxn("NetStats", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT COUNT(key) AS num FROM node")
		err := row.Scan(&mapSize)
		if err != nil {
			if err != sql.ErrNoRows {
				log.Printf("[Store] NetStats: %v", err)
			}
			return nil
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) coreNodeList(tx *sql.Tx) (res []spec.CoreNode, err error) {
	rows, err := tx.Query("SELECT address,time,services FROM core")
	if err != nil {
		return nil, fmt.Errorf("[Store] coreNodeList: query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var addr []byte
		var time int64
		var services uint64
		err := rows.Scan(&addr, &time, &services)
		if err != nil {
			return nil, fmt.Errorf("[Store] coreNodeList: scanning row: %v", err)
		}
		s_adr, err := dnet.AddressFromBytes(addr)
		if err != nil {
			return nil, fmt.Errorf("[Store] bad node address: %v", err)
		}
		res = append(res, spec.CoreNode{
			Address:  s_adr.String(),
			Time:     time,
			Services: services,
		})
	}
	if err = rows.Err(); err != nil { // docs say this check is required!
		return nil, fmt.Errorf("[Store] query: %v", err)
	}
	return
}

func (s SQLiteStoreCtx) netNodeList(tx *sql.Tx) (res []spec.NetNode, err error) {
	// use payload because it contains all the channels
	rows, err := tx.Query("SELECT key,payload,time FROM node")
	if err != nil {
		return nil, fmt.Errorf("[Store] netNodeList: query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pubkey []byte
		var payload []byte
		var unixTime int64
		err := rows.Scan(&pubkey, &payload, &unixTime)
		if err != nil {
			return nil, fmt.Errorf("[Store] netNodeList: scanning row: %v", err)
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
		return nil, fmt.Errorf("[Store] query: %v", err)
	}
	return
}

func (s SQLiteStoreCtx) NodeList() (res spec.NodeListRes, err error) {
	err = s.doTxn("NodeList", func(tx *sql.Tx) error {
		res.Core, err = s.coreNodeList(tx)
		if err != nil {
			return err
		}
		res.Net, err = s.netNodeList(tx)
		if err != nil {
			return err
		}
		return nil
	})
	return
}

// TrimNodes expires both network and core nodes after 30 days.
//
// To take account of the possibility that this software has not
// been run in the last 30 days (which would result in immediately
// expiring all nodes in the database) we use a system where:
//
// We keep a day counter that we increment once per day.
// All nodes, when updated, store the current day counter + 30.
// Nodes are expired once their stored day-count is < today.
//
// This causes node-expiry to lag by the number of offline days.
func (s SQLiteStoreCtx) TrimNodes() (advanced bool, remCore int64, remNode int64, err error) {
	err = s.doTxn("TrimNodes", func(tx *sql.Tx) error {
		// check if date has changed
		row := tx.QueryRow("SELECT dayc,last FROM config LIMIT 1")
		var dayc int64
		var last int64
		err := row.Scan(&dayc, &last)
		if err != nil {
			return fmt.Errorf("TrimNodes: SELECT config: %v", err)
		}
		today := unixDayStamp()
		if last != today {
			// advance the day-count and save unix-daystamp
			dayc += 1
			advanced = true
			_, err := tx.Exec("UPDATE config SET dayc=?,last=?", dayc, today)
			if err != nil {
				return fmt.Errorf("TrimNodes: UPDATE config: %v", err)
			}
			// expire core nodes
			res, err := tx.Exec("DELETE FROM core WHERE dayc < ?", dayc)
			if err != nil {
				return fmt.Errorf("TrimNodes: DELETE core: %v", err)
			}
			remCore, err = res.RowsAffected()
			if err != nil {
				return fmt.Errorf("TrimNodes: rows-affected: %v", err)
			}
			// expire net nodes
			res, err = tx.Exec("DELETE FROM node WHERE dayc < ?", dayc)
			if err != nil {
				return fmt.Errorf("TrimNodes: DELETE node: %v", err)
			}
			remNode, err = res.RowsAffected()
			if err != nil {
				return fmt.Errorf("TrimNodes: rows-affected: %v", err)
			}
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) AddCoreNode(address Address, unixTimeSec int64, services uint64) error {
	return s.doTxn("AddCoreNode", func(tx *sql.Tx) error {
		addrKey := address.ToBytes()
		res, err := tx.Exec("UPDATE core SET time=?, services=?, dayc=30+(SELECT dayc FROM config LIMIT 1) WHERE address=?", unixTimeSec, services, addrKey)
		if err != nil {
			return fmt.Errorf("update: %v", err)
		}
		num, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows-affected: %v", err)
		}
		if num == 0 {
			_, e := tx.Exec("INSERT INTO core (address, time, services, isnew, dayc) VALUES (?1,?2,?3,true,30+(SELECT dayc FROM config LIMIT 1))",
				addrKey, unixTimeSec, services)
			if e != nil {
				return fmt.Errorf("insert: %v", e)
			}
		}
		return nil
	})
}

func (s SQLiteStoreCtx) UpdateCoreTime(address Address) (err error) {
	return s.doTxn("UpdateCoreTime", func(tx *sql.Tx) error {
		addrKey := address.ToBytes()
		unixTimeSec := time.Now().Unix()
		_, err := tx.Exec("UPDATE core SET time=?, dayc=30+(SELECT dayc FROM config LIMIT 1) WHERE address=?", unixTimeSec, addrKey)
		if err != nil {
			return fmt.Errorf("update: %v", err)
		}
		return nil
	})
}

func (s SQLiteStoreCtx) ChooseCoreNode() (res Address, err error) {
	err = s.doTxn("ChooseCoreNode", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT address FROM core WHERE isnew=TRUE ORDER BY RANDOM() LIMIT 1")
		var addr []byte
		err := row.Scan(&addr)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				row = tx.QueryRow("SELECT address FROM core WHERE isnew=FALSE ORDER BY RANDOM() LIMIT 1")
				err = row.Scan(&addr)
				if err != nil {
					return fmt.Errorf("query-not-new: %v", err)
				}
			} else {
				return fmt.Errorf("query-is-new: %v", err)
			}
		}
		res, err = dnet.AddressFromBytes(addr)
		if err != nil {
			return fmt.Errorf("invalid address: %v", err)
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) SampleCoreNodes() (res []Address, err error) {
	err = s.doTxn("SampleCoreNodes", func(tx *sql.Tx) error {
		return nil
	})
	return
}

func (s SQLiteStoreCtx) GetAnnounce() (payload []byte, sig []byte, time int64, err error) {
	err = s.doTxn("GetAnnounce", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT payload, sig, time FROM announce LIMIT 1")
		e := row.Scan(&payload, &sig, &time)
		if e != nil {
			if !errors.Is(e, sql.ErrNoRows) {
				err = fmt.Errorf("query: %v", e)
			}
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) SetAnnounce(payload []byte, sig []byte, time int64) error {
	return s.doTxn("SetAnnounce", func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE announce SET payload=?,sig=?,time=?", payload, sig, time)
		if err != nil {
			return err
		}
		num, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if num == 0 {
			_, err = tx.Exec("INSERT INTO announce (payload,sig,time) VALUES (?,?,?)", payload, sig, time)
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

func (s SQLiteStoreCtx) UpdateNetTime(key spec.PubKey) (err error) {
	err = s.doTxn("SampleCoreNodes", func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE node SET dayc=30+(SELECT dayc FROM config LIMIT 1) WHERE key=?", key)
		if e != nil {
			return fmt.Errorf("update: %v", e)
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) ChooseNetNode() (res spec.NodeInfo, err error) {
	err = s.doTxn("SampleCoreNodes", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT key,address FROM node ORDER BY RANDOM() LIMIT 1")
		var key []byte
		var addr []byte
		err := row.Scan(&key, &addr)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			} else {
				return fmt.Errorf("query: %v", err)
			}
		}
		if len(key) != 32 {
			return fmt.Errorf("invalid node key: %v (should be 32 bytes)", hex.EncodeToString(key))
		}
		res.PubKey = ([32]byte)(key)
		res.Addr, err = dnet.AddressFromBytes(addr)
		if err != nil {
			return fmt.Errorf("invalid address: %v", err)
		}
		return nil
	})
	return
}

func (s SQLiteStoreCtx) SampleNetNodes() (res []spec.NodeInfo, err error) {
	err = s.doTxn("SampleCoreNodes", func(tx *sql.Tx) error {
		return nil
	})
	return
}

func (s SQLiteStoreCtx) SampleNodesByChannel(channels []dnet.Tag4CC, exclude []spec.PubKey) (res []spec.NodeInfo, err error) {
	err = s.doTxn("SampleCoreNodes", func(tx *sql.Tx) error {
		return nil
	})
	return
}

func (s SQLiteStoreCtx) SampleNodesByIP(ipaddr net.IP, exclude []spec.PubKey) (res []spec.NodeInfo, err error) {
	err = s.doTxn("SampleCoreNodes", func(tx *sql.Tx) error {
		return nil
	})
	return
}
