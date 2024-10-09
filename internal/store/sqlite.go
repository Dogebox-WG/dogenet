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
	db  *sql.DB
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
CREATE TABLE IF NOT EXISTS migration (
	version INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS config (
	dayc INTEGER NOT NULL,
	last INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS announce (
	payload BLOB NOT NULL,
	sig BLOB NOT NULL,
	time INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS channels (
	chan INTEGER NOT NULL PRIMARY KEY,
	dayc INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS core (
	address BLOB NOT NULL PRIMARY KEY,
	time INTEGER NOT NULL,
	services INTEGER NOT NULL,
	isnew BOOLEAN NOT NULL,
	dayc INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS core_time_i ON core (time);
CREATE INDEX IF NOT EXISTS core_isnew_i ON core (isnew);
CREATE TABLE IF NOT EXISTS node (
	key BLOB NOT NULL PRIMARY KEY,
	address BLOB NOT NULL,
	time INTEGER NOT NULL,
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

const SQL_MIGRATION_v2 string = `
ALTER TABLE announce ADD COLUMN owner BLOB
`

var MIGRATIONS = []struct {
	ver   int
	query string
}{
	{2, SQL_MIGRATION_v2},
}

// NewSQLiteStore returns a spec.Store implementation that uses SQLite
func NewSQLiteStore(fileName string, ctx context.Context) (spec.Store, error) {
	backend := "sqlite3"
	db, err := sql.Open(backend, fileName)
	store := &SQLiteStore{db: db, ctx: ctx}
	if err != nil {
		return store, dbErr(err, "opening database")
	}
	if backend == "sqlite3" {
		// limit concurrent access until we figure out a way to start transactions
		// with the BEGIN CONCURRENT statement in Go. Avoids "database locked" errors.
		db.SetMaxOpenConns(1)
	}
	err = store.initSchema()
	return store, err
}

func (s *SQLiteStore) Close() {
	s.db.Close()
}

func (s *SQLiteStore) initSchema() error {
	return s.doTxn("init schema", func(tx *sql.Tx) error {
		// apply migrations
		verRow := tx.QueryRow("SELECT version FROM migration LIMIT 1")
		var version int
		err := verRow.Scan(&version)
		if err != nil {
			// first-time database init.
			// init schema (idempotent)
			_, err := tx.Exec(SQL_SCHEMA)
			if err != nil {
				return dbErr(err, "creating database schema")
			}
			// set up version table (idempotent)
			err = tx.QueryRow("SELECT version FROM migration LIMIT 1").Scan(&version)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					version = 1
					_, err = tx.Exec("INSERT INTO migration (version) VALUES (?)", version)
					if err != nil {
						return dbErr(err, "updating version")
					}
				} else {
					return dbErr(err, "querying version")
				}
			}
			// set up config table (idempotent)
			// this is application-specific.
			var dayc int
			err = tx.QueryRow("SELECT dayc FROM config LIMIT 1").Scan(&dayc)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					_, err = tx.Exec("INSERT INTO config (dayc,last) VALUES (1,?)", unixDayStamp())
					if err != nil {
						return dbErr(err, "inserting config row")
					}
				} else {
					return dbErr(err, "querying config")
				}
			}
		}
		initVer := version
		for _, m := range MIGRATIONS {
			if version < m.ver {
				_, err = tx.Exec(m.query)
				if err != nil {
					return dbErr(err, fmt.Sprintf("applying migration %v", m.ver))
				}
				version = m.ver
			}
		}
		if version != initVer {
			_, err = tx.Exec("UPDATE migration SET version=?", version)
			if err != nil {
				return dbErr(err, "updating version")
			}
		}
		return nil
	})
}

func (s *SQLiteStore) WithCtx(ctx context.Context) spec.Store {
	return &SQLiteStore{
		db:  s.db,
		ctx: ctx,
	}
}

// The number of whole days since the unix epoch.
func unixDayStamp() int64 {
	return time.Now().Unix() / spec.SecondsPerDay
}

func IsConflict(err error) bool {
	if sqErr, isSq := err.(sqlite3.Error); isSq {
		if sqErr.Code == sqlite3.ErrBusy || sqErr.Code == sqlite3.ErrLocked {
			return true
		}
	}
	return false
}

func (s SQLiteStore) doTxn(name string, work func(tx *sql.Tx) error) error {
	limit := 120
	for {
		tx, err := s.db.Begin()
		if err != nil {
			if IsConflict(err) {
				s.Sleep(250 * time.Millisecond)
				limit--
				if limit != 0 {
					continue
				}
			}
			return dbErr(err, "cannot begin transaction: "+name)
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
			return err
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
			return dbErr(err, "cannot commit: "+name)
		}
		return nil
	}
}

func (s SQLiteStore) Sleep(dur time.Duration) {
	select {
	case <-s.ctx.Done():
	case <-time.After(dur):
	}
}

func dbErr(err error, where string) error {
	if errors.Is(err, spec.NotFoundError) {
		return err
	}
	if sqErr, isSq := err.(sqlite3.Error); isSq {
		if sqErr.Code == sqlite3.ErrConstraint {
			// MUST detect 'AlreadyExists' to fulfil the API contract!
			// Constraint violation, e.g. a duplicate key.
			return spec.WrapErr(spec.AlreadyExists, "SQLiteStore: already-exists", err)
		}
		if sqErr.Code == sqlite3.ErrBusy || sqErr.Code == sqlite3.ErrLocked {
			// SQLite has a single-writer policy, even in WAL (write-ahead) mode.
			// SQLite will return BUSY if the database is locked by another connection.
			// We treat this as a transient database conflict, and the caller should retry.
			return spec.WrapErr(spec.DBConflict, "SQLiteStore: db-conflict", err)
		}
	}
	return spec.WrapErr(spec.DBProblem, fmt.Sprintf("SQLiteStore: db-problem: %s", where), err)
}

// STORE INTERFACE

func (s SQLiteStore) CoreStats() (mapSize int, newNodes int, err error) {
	err = s.doTxn("CoreStats", func(tx *sql.Tx) error {
		row := tx.QueryRow("WITH t AS (SELECT COUNT(address) AS num, 1 AS rn FROM core), u AS (SELECT COUNT(address) AS isnew, 1 AS rn FROM core WHERE isnew=TRUE) SELECT t.num, u.isnew FROM t INNER JOIN u ON t.rn=u.rn")
		err := row.Scan(&mapSize, &newNodes)
		if err != nil {
			// special case: always return nil (no stats) errors.
			if err != sql.ErrNoRows {
				log.Printf("[Store] CoreStats: %v", err)
			}
			return nil
		}
		return nil
	})
	return
}

func (s SQLiteStore) NetStats() (mapSize int, err error) {
	err = s.doTxn("NetStats", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT COUNT(key) AS num FROM node")
		err := row.Scan(&mapSize)
		if err != nil {
			// special case: always return nil (no stats) errors.
			if err != sql.ErrNoRows {
				log.Printf("[Store] NetStats: %v", err)
			}
			return nil
		}
		return nil
	})
	return
}

func (s SQLiteStore) coreNodeList(tx *sql.Tx) (res []spec.CoreNode, err error) {
	rows, err := tx.Query("SELECT address,CAST(time AS INTEGER),services FROM core")
	if err != nil {
		return nil, fmt.Errorf("[Store] coreNodeList: query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var addr []byte
		var unixTime int64
		var services uint64
		err := rows.Scan(&addr, &unixTime, &services)
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
			Time:     unixTime,
			Services: services,
		})
	}
	if err = rows.Err(); err != nil { // docs say this check is required!
		return nil, fmt.Errorf("[Store] query: %v", err)
	}
	return
}

func (s SQLiteStore) netNodeList(tx *sql.Tx) (res []spec.NetNode, err error) {
	// use payload because it contains all the channels
	rows, err := tx.Query("SELECT key,payload,CAST(time AS INTEGER) FROM node")
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

func (s SQLiteStore) NodeList() (res spec.NodeListRes, err error) {
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

// TrimNodes expires records after N days.
//
// To take account of the possibility that this software has not
// been run in the last N days (which would result in immediately
// expiring all records) we use a system where:
//
// We keep a day counter that we increment once per day.
// All records, when updated, store the current day counter + N.
// Records expire once their stored day-count is < today.
//
// This causes expiry to lag by the number of offline days.
func (s SQLiteStore) TrimNodes() (advanced bool, remCore int64, remNode int64, err error) {
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

			// expire net nodes
			res, err := tx.Exec("DELETE FROM node WHERE dayc < ?", dayc)
			if err != nil {
				return fmt.Errorf("TrimNodes: DELETE node: %v", err)
			}
			remNode, err = res.RowsAffected()
			if err != nil {
				return fmt.Errorf("TrimNodes: rows-affected: %v", err)
			}

			// expire channels
			res, err = tx.Exec("DELETE FROM channels WHERE dayc < ?", dayc)
			if err != nil {
				return fmt.Errorf("TrimNodes: DELETE channel: %v", err)
			}
			remNode, err = res.RowsAffected()
			if err != nil {
				return fmt.Errorf("TrimNodes: rows-affected: %v", err)
			}
		}
		// expire core nodes,
		// which don't use the day-count policy
		// because a +/- 1 day tolerance is too much
		unixTimeSec := time.Now().Unix()
		expireBefore := unixTimeSec - spec.MaxCoreNodeDays*spec.SecondsPerDay
		res, err := tx.Exec("DELETE FROM core WHERE time < ?", expireBefore)
		if err != nil {
			return fmt.Errorf("TrimNodes: DELETE core: %v", err)
		}
		remCore, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("TrimNodes: rows-affected: %v", err)
		}
		return nil
	})
	return
}

func (s SQLiteStore) AddCoreNode(address Address, unixTimeSec int64, services uint64) error {
	return s.doTxn("AddCoreNode", func(tx *sql.Tx) error {
		addrKey := address.ToBytes()
		res, err := tx.Exec("UPDATE core SET time=?, services=? WHERE address=?", unixTimeSec, services, addrKey)
		if err != nil {
			return fmt.Errorf("update: %v", err)
		}
		num, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows-affected: %v", err)
		}
		if num == 0 {
			_, e := tx.Exec("INSERT INTO core (address, time, services, isnew, dayc) VALUES (?1,?2,?3,true,0)",
				addrKey, unixTimeSec, services)
			if e != nil {
				return fmt.Errorf("insert: %v", e)
			}
		}
		return nil
	})
}

func (s SQLiteStore) UpdateCoreTime(address Address) (err error) {
	return s.doTxn("UpdateCoreTime", func(tx *sql.Tx) error {
		addrKey := address.ToBytes()
		unixTimeSec := time.Now().Unix()
		_, err := tx.Exec("UPDATE core SET time=? WHERE address=?", unixTimeSec, addrKey)
		if err != nil {
			return fmt.Errorf("update: %v", err)
		}
		return nil
	})
}

func (s SQLiteStore) ChooseCoreNode() (res Address, err error) {
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

func (s SQLiteStore) GetAnnounce() (payload []byte, sig []byte, time int64, owner []byte, err error) {
	err = s.doTxn("GetAnnounce", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT payload,sig,time,owner FROM announce LIMIT 1")
		e := row.Scan(&payload, &sig, &time, &owner)
		if e != nil {
			if !errors.Is(e, sql.ErrNoRows) {
				return fmt.Errorf("query: %v", e)
			}
		}
		return nil
	})
	return
}

func (s SQLiteStore) SetAnnounce(payload []byte, sig []byte, time int64) error {
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
			_, err = tx.Exec("INSERT INTO announce (payload,sig,time,owner) VALUES (?,?,?)", payload, sig, time)
		}
		return err
	})
}

func (s SQLiteStore) SetAnnounceOwner(owner []byte) error {
	return s.doTxn("SetAnnounceOwner", func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE announce SET owner=?", owner)
		if err != nil {
			return err
		}
		num, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if num == 0 {
			_, err = tx.Exec("INSERT INTO announce (payload,sig,time,owner) VALUES (?,?,?,?)", []byte{}, []byte{}, 0, owner)
		}
		return err
	})
}

func (s SQLiteStore) GetChannels() (channels []dnet.Tag4CC, err error) {
	err = s.doTxn("GetChannels", func(tx *sql.Tx) error {
		rows, err := tx.Query("SELECT chan FROM channels")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ch uint32
			err = rows.Scan(&ch)
			if err != nil {
				return dbErr(err, "GetChannels: scanning row")
			}
			channels = append(channels, dnet.Tag4CC(ch))
		}
		if err = rows.Err(); err != nil { // docs say this check is required!
			return dbErr(err, "GetChannels: querying channels")
		}
		return nil
	})
	return
}

func (s SQLiteStore) AddChannel(channel dnet.Tag4CC) error {
	return s.doTxn("AddChannel", func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE channels SET dayc=7+(SELECT dayc FROM config LIMIT 1) WHERE chan=?", channel)
		if err != nil {
			return err
		}
		num, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if num == 0 {
			_, err = tx.Exec("INSERT INTO channels (chan,dayc) VALUES (?,7+(SELECT dayc FROM config LIMIT 1))", channel)
		}
		return err
	})
}

// const add_netnode_psql = "INSERT INTO node (key, address, time, owner, payload, sig, dayc) VALUES (?1,?2,?3,?4,?5,?6,30+(SELECT dayc FROM config LIMIT 1)) ON CONFLICT ON CONSTRAINT node_key DO UPDATE SET address=?2, time=?3, owner=?4, payload=?5, sig=?6, dayc=30+(SELECT dayc FROM config LIMIT 1)"
// const add_netnode_sqlite = "INSERT INTO node (key, address, time, owner, payload, sig, dayc) VALUES (?1,?2,?3,?4,?5,?6,30+(SELECT dayc FROM config LIMIT 1)) ON CONFLICT REPLACE RETURNING oid"

func (s SQLiteStore) AddNetNode(key []byte, address Address, time int64, owner []byte, channels []dnet.Tag4CC, payload []byte, sig []byte) (changed bool, err error) {
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

func (s SQLiteStore) UpdateNetTime(key []byte) (err error) {
	err = s.doTxn("UpdateNetTime", func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE node SET dayc=30+(SELECT dayc FROM config LIMIT 1) WHERE key=?", key)
		if e != nil {
			return fmt.Errorf("update: %v", e)
		}
		return nil
	})
	return
}

func (s SQLiteStore) ChooseNetNode() (res spec.NodeInfo, err error) {
	err = s.doTxn("ChooseNetNode", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT key,address FROM node WHERE oid IN (SELECT oid FROM node ORDER BY RANDOM() LIMIT 1)")
		var key []byte
		var addr []byte
		err := row.Scan(&key, &addr)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return spec.NotFoundError
			} else {
				return fmt.Errorf("query: %v", err)
			}
		}
		if len(key) != 32 {
			return fmt.Errorf("invalid node key: %v (should be 32 bytes)", hex.EncodeToString(key))
		}
		res.PubKey = *(*[32]byte)(key) // Go 1.17
		res.Addr, err = dnet.AddressFromBytes(addr)
		if err != nil {
			return fmt.Errorf("invalid address: %v", err)
		}
		return nil
	})
	return
}

func (s SQLiteStore) ChooseNetNodeMsg() (r spec.NodeRecord, err error) {
	err = s.doTxn("ChooseNetNodeMsg", func(tx *sql.Tx) error {
		row := tx.QueryRow("SELECT key,payload,sig FROM node WHERE oid IN (SELECT oid FROM node ORDER BY RANDOM() LIMIT 1)")
		err := row.Scan(&r.PubKey, &r.Payload, &r.Sig)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return spec.NotFoundError
			} else {
				return fmt.Errorf("query: %v", err)
			}
		}
		return nil
	})
	return
}

func (s SQLiteStore) SampleNodesByChannel(channels []dnet.Tag4CC, exclude [][]byte) (res []spec.NodeInfo, err error) {
	err = s.doTxn("SampleNodesByChannel", func(tx *sql.Tx) error {
		return nil
	})
	return
}

func (s SQLiteStore) SampleNodesByIP(ipaddr net.IP, exclude [][]byte) (res []spec.NodeInfo, err error) {
	err = s.doTxn("SampleNodesByIP", func(tx *sql.Tx) error {
		return nil
	})
	return
}
