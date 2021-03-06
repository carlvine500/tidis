//
// ttl.go
// Copyright (C) 2018 YanMing <yming0221@gmail.com>
//
// Distributed under terms of the MIT license.
//

package tidis

import (
	"math"
	"time"

	"github.com/pingcap/tidb/kv"
	"github.com/yongman/go/log"
	ti "github.com/yongman/tidis/store/tikv"
	"github.com/yongman/tidis/terror"
)

// ttl for user key checker and operater

type ttlChecker struct {
	dataType   byte
	maxPerLoop int
	interval   int
	tdb        *Tidis
}

func NewTTLChecker(datatype byte, max, interval int, tdb *Tidis) *ttlChecker {
	return &ttlChecker{
		dataType:   datatype,
		maxPerLoop: max,
		interval:   interval,
		tdb:        tdb,
	}
}

func (ch *ttlChecker) Run() {
	c := time.Tick(time.Duration(ch.interval) * time.Millisecond)
	for _ = range c {
		if ch.dataType == TSTRING {
			startKey := TMSEncoder([]byte{0}, 0)
			endKey := TMSEncoder([]byte{0}, math.MaxInt64)

			f := func(txn1 interface{}) (interface{}, error) {
				txn, ok := txn1.(kv.Transaction)
				if !ok {
					return 0, terror.ErrBackendType
				}

				var loops int

				ss := txn.GetSnapshot()
				// create iterater
				it, err := ti.NewIterator(startKey, endKey, ss, false)
				if err != nil {
					return 0, err
				}

				loops = ch.maxPerLoop
				for loops > 0 && it.Valid() {
					// decode user key
					key, ts, err := TMSDecoder(it.Key())
					if err != nil {
						return 0, err
					}
					if ts > uint64(time.Now().UnixNano()/1000/1000) {
						// no key expired
						break
					}
					// delete ttlmetakey ttldatakey key
					tDataKey := TDSEncoder(key)
					sKey := SEncoder(key)

					if err = txn.Delete(it.Key()); err != nil {
						return 0, err
					}
					if err = txn.Delete(tDataKey); err != nil {
						return 0, err
					}
					if err = txn.Delete(sKey); err != nil {
						return 0, err
					}

					it.Next()
					loops--
				}
				return ch.maxPerLoop - loops, nil
			}

			// exe txn
			v, err := ch.tdb.db.BatchInTxn(f)
			if err != nil {
				log.Warnf("ttl checker decode key failed, %s", err.Error())
			}
			log.Debugf("ttl checker delete %d keys in this loop", v.(int))
		}
	}
}
