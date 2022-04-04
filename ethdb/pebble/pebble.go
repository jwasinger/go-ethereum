// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package pebble implements the key-value database layer based on pebble.
package pebble

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// minCache is the minimum amount of memory in megabytes to allocate to leveldb
	// read and write caching, split half and half.
	minCache = 16

	// minHandles is the minimum number of files handles to allocate to the open
	// database files.
	minHandles = 16

	// metricsGatheringInterval specifies the interval to retrieve leveldb database
	// compaction, io and pause stats to report to the user.
	metricsGatheringInterval = 3 * time.Second
)

// Database is a persistent key-value store based on the pebble storage engine.
// Apart from basic data storage functionality it also supports batch writes and
// iterating over the keyspace in binary-alphabetical order.
type Database struct {
	fn string     // filename for reporting
	db *pebble.DB // Underlying pebble storage engine

	compTimeMeter      metrics.Meter // Meter for measuring the total time spent in database compaction
	compReadMeter      metrics.Meter // Meter for measuring the data read during compaction
	compWriteMeter     metrics.Meter // Meter for measuring the data written during compaction
	writeDelayNMeter   metrics.Meter // Meter for measuring the write delay number due to database compaction
	writeDelayMeter    metrics.Meter // Meter for measuring the write delay duration due to database compaction
	diskSizeGauge      metrics.Gauge // Gauge for tracking the size of all the levels in the database
	diskReadMeter      metrics.Meter // Meter for measuring the effective amount of data read
	diskWriteMeter     metrics.Meter // Meter for measuring the effective amount of data written
	memCompGauge       metrics.Gauge // Gauge for tracking the number of memory compaction
	level0CompGauge    metrics.Gauge // Gauge for tracking the number of table compaction in level0
	nonlevel0CompGauge metrics.Gauge // Gauge for tracking the number of table compaction in non0 level
	seekCompGauge      metrics.Gauge // Gauge for tracking the number of table compaction caused by read opt

	quitLock sync.Mutex      // Mutex protecting the quit channel access
	quitChan chan chan error // Quit channel to stop the metrics collection before closing the database

	log log.Logger // Contextual logger tracking the database path

	activeCompactionCount int

	compactionStartTime time.Time
	compactionCount     int64
	compactionTime      int64

	writeDelayStartTime time.Time
	writeDelayCount     int64
	writeDelayTime      int64
}

func (d *Database) OnCompactionBegin(info pebble.CompactionInfo) {
	if d.activeCompactionCount == 0 {
		d.compactionStartTime = time.Now()
	}

	d.activeCompactionCount++
}

// TODO check that this is called synchronously when there are multiple concurrent compactions
func (d *Database) OnCompactionEnd(info pebble.CompactionInfo) {
	if d.activeCompactionCount == 1 {
		atomic.AddInt64(&d.compactionTime, int64(time.Since(d.compactionStartTime)))
	} else if d.activeCompactionCount == 0 {
		panic("should not happen")
	}

	d.compactionCount++
	d.activeCompactionCount--
}

func (d *Database) OnWriteStallBegin(b pebble.WriteStallBeginInfo) {
	d.writeDelayStartTime = time.Now()
}

func (d *Database) OnWriteStallEnd() {
	atomic.AddInt64(&d.writeDelayTime, int64(time.Since(d.writeDelayStartTime)))
}

// New returns a wrapped LevelDB object. The namespace is the prefix that the
// metrics reporting should use for surfacing internal stats.
func New(file string, cache int, handles int, namespace string, readonly bool) (*Database, error) {
	var ldb *Database
	// Ensure we have some minimal caching and file guarantees
	if cache < minCache {
		cache = minCache
	}
	if handles < minHandles {
		handles = minHandles
	}
	logger := log.New("database", file)
	logger.Info("Allocated cache and file handles", "cache", common.StorageSize(cache*1024*1024), "handles", handles)

	// TODO race between setting ldb below and modifying it here?
	eventListener := pebble.EventListener{
		CompactionBegin: func(info pebble.CompactionInfo) {
			ldb.OnCompactionBegin(info)
		},
		CompactionEnd: func(info pebble.CompactionInfo) {
			ldb.OnCompactionEnd(info)
		},
		WriteStallBegin: func(info pebble.WriteStallBeginInfo) {
			ldb.OnWriteStallBegin(info)
		},
		WriteStallEnd: func() {
			ldb.OnWriteStallEnd()
		},
	}
	// Open the db and recover any potential corruptions
	db, err := pebble.Open(file, &pebble.Options{
		// Pebble has a single combined cache area and the write
		// buffers are taken from this too. Assign all available
		// memory allowance for cache.
		Cache:        pebble.NewCache(int64(cache * 1024 * 1024)),
		MaxOpenFiles: handles,
		// The size of memory table(as well as the write buffer).
		// Note, there may have more than two memory tables in the system.
		// MemTableStopWritesThreshold can be configured to avoid the memory abuse.
		MemTableSize: cache * 1024 * 1024 / 4,
		// The default compaction concurrency(1 thread),
		// Here use all available CPUs for faster compaction.
		MaxConcurrentCompactions: runtime.NumCPU(),
		// Per-level options. Options for at least one level must be specified. The
		// options for the last level are used for all subsequent levels.
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		},
		ReadOnly:      readonly,
		EventListener: eventListener,
	})
	if err != nil {
		return nil, err
	}
	// Assemble the wrapper with all the registered metrics
	ldb = &Database{
		fn:       file,
		db:       db,
		log:      logger,
		quitChan: make(chan chan error),
	}
	ldb.compTimeMeter = metrics.NewRegisteredMeter(namespace+"compact/time", nil)
	ldb.compReadMeter = metrics.NewRegisteredMeter(namespace+"compact/input", nil)
	ldb.compWriteMeter = metrics.NewRegisteredMeter(namespace+"compact/output", nil)
	ldb.diskSizeGauge = metrics.NewRegisteredGauge(namespace+"disk/size", nil)
	ldb.diskReadMeter = metrics.NewRegisteredMeter(namespace+"disk/read", nil)
	ldb.diskWriteMeter = metrics.NewRegisteredMeter(namespace+"disk/write", nil)
	ldb.writeDelayMeter = metrics.NewRegisteredMeter(namespace+"compact/writedelay/duration", nil)
	ldb.writeDelayNMeter = metrics.NewRegisteredMeter(namespace+"compact/writedelay/counter", nil)
	ldb.memCompGauge = metrics.NewRegisteredGauge(namespace+"compact/memory", nil)
	ldb.level0CompGauge = metrics.NewRegisteredGauge(namespace+"compact/level0", nil)
	ldb.nonlevel0CompGauge = metrics.NewRegisteredGauge(namespace+"compact/nonlevel0", nil)
	ldb.seekCompGauge = metrics.NewRegisteredGauge(namespace+"compact/seek", nil)

	// Start up the metrics gathering and return
	go ldb.meter(metricsGatheringInterval)
	return ldb, nil
}

// Close stops the metrics collection, flushes any pending data to disk and closes
// all io accesses to the underlying key-value store.
func (db *Database) Close() error {
	db.quitLock.Lock()
	defer db.quitLock.Unlock()

	if db.quitChan != nil {
		errc := make(chan error)
		db.quitChan <- errc
		if err := <-errc; err != nil {
			db.log.Error("Metrics collection failed", "err", err)
		}
		db.quitChan = nil
	}
	return db.db.Close()
}

// Has retrieves if a key is present in the key-value store.
func (db *Database) Has(key []byte) (bool, error) {
	_, closer, err := db.db.Get(key)
	if err == pebble.ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	closer.Close()
	return true, nil
}

// Get retrieves the given key if it's present in the key-value store.
func (db *Database) Get(key []byte) ([]byte, error) {
	dat, closer, err := db.db.Get(key)
	if err != nil {
		return nil, err
	}
	ret := make([]byte, len(dat))
	copy(ret, dat)
	closer.Close()
	return ret, nil
}

// Put inserts the given value into the key-value store.
func (db *Database) Put(key []byte, value []byte) error {
	return db.db.Set(key, value, pebble.NoSync)
}

// Delete removes the key from the key-value store.
func (db *Database) Delete(key []byte) error {
	return db.db.Delete(key, nil)
}

// NewBatch creates a write-only key-value store that buffers changes to its host
// database until a final write is called.
func (db *Database) NewBatch() ethdb.Batch {
	return &batch{
		b: db.db.NewBatch(),
	}
}

// NewBatchWithSize creates a write-only database batch with pre-allocated buffer.
// TODO can't do this with pebble.  Batches are allocated in a pool so maybe this doesn't matter?
func (db *Database) NewBatchWithSize(_ int) ethdb.Batch {
	return &batch{
		b: db.db.NewBatch(),
	}
}

// snapshot wraps a pebble snapshot for implementing the Snapshot interface.
type snapshot struct {
	db *pebble.Snapshot
}

// Has retrieves if a key is present in the snapshot backing by a key-value
// data store.
func (snap *snapshot) Has(key []byte) (bool, error) {
	_, closer, err := snap.db.Get(key)
	defer closer.Close()

	if err != nil {
		if err != pebble.ErrNotFound {
			return false, err
		} else {
			return false, nil
		}
	}
	return true, nil
}

// Get retrieves the given key if it's present in the snapshot backing by
// key-value data store.
func (snap *snapshot) Get(key []byte) ([]byte, error) {
	val, closer, err := snap.db.Get(key)
	defer closer.Close()

	if err != nil {
		return nil, err
	}
	return val, nil
}

// Release releases associated resources. Release should always succeed and can
// be called multiple times without causing error.
func (snap *snapshot) Release() {
	snap.db.Close()
}

// NewSnapshot creates a database snapshot based on the current state.
// The created snapshot will not be affected by all following mutations
// happened on the database.
// Note don't forget to release the snapshot once it's used up, otherwise
// the stale data will never be cleaned up by the underlying compactor.
func (db *Database) NewSnapshot() (ethdb.Snapshot, error) {
	snap := db.db.NewSnapshot()
	return &snapshot{db: snap}, nil
}

// NewIterator creates a binary-alphabetical iterator over a subset
// of database content with a particular key prefix, starting at a particular
// initial key (or after, if it does not exist).
func (db *Database) NewIterator(prefix []byte, start []byte) ethdb.Iterator {
	iterRange := bytesPrefixRange(prefix, start)
	iter := db.db.NewIter(&pebble.IterOptions{
		LowerBound: iterRange.Start,
		UpperBound: iterRange.Limit,
	})
	iter.First()
	return &pebbleIterator{iter: iter, moved: true}
}

// Stat returns a particular internal stat of the database.
func (db *Database) Stat(property string) (string, error) {
	return "", nil // TODO implement this
}

// Compact flattens the underlying data store for the given key range. In essence,
// deleted and overwritten versions are discarded, and the data is rearranged to
// reduce the cost of operations needed to access them.
//
// A nil start is treated as a key before all keys in the data store; a nil limit
// is treated as a key after all keys in the data store. If both is nil then it
// will compact entire data store.
func (db *Database) Compact(start []byte, limit []byte) error {
	return db.db.Compact(start, limit, false)
}

// Path returns the path to the database directory.
func (db *Database) Path() string {
	return db.fn
}

// meter periodically retrieves internal leveldb counters and reports them to
// the metrics subsystem.
func (db *Database) meter(refresh time.Duration) {
	var errc chan error
	timer := time.NewTimer(refresh)
	defer timer.Stop()

	// Create storage and warning log tracer for write delay.
	var (
		//delaystats       [2]int64
		compactionCounts [2]int64
		compactionTimes  [2]int64
		writeDelayTimes  [2]int64
		writeDelayCounts [2]int64
		compactionWrites [2]int64
		compactionReads  [2]int64

		//lastWritePaused time.Time
	)

	// Iterate ad infinitum and collect the stats
	for i := 1; errc == nil; i++ {
		metrics := db.db.Metrics()

		compactionCount := atomic.LoadInt64(&db.compactionCount)
		compactionTime := atomic.LoadInt64(&db.compactionTime)
		writeDelayCount := atomic.LoadInt64(&db.writeDelayCount)
		writeDelayTime := atomic.LoadInt64(&db.writeDelayTime)

		writeDelayTimes[i%2] = writeDelayTime
		writeDelayCounts[i%2] = writeDelayCount
		compactionCounts[i%2] = compactionCount
		compactionTimes[i%2] = compactionTime

		var (
			compactionWrite int64
			compactionRead  int64
		)

		for i := 0; i < len(metrics.Levels); i++ {
			compactionWrite += int64(metrics.Levels[i].BytesCompacted)
			compactionRead += int64(metrics.Levels[i].BytesRead)
		}

		compactionWrites[i%2] = compactionWrite
		compactionReads[i%2] = compactionRead

		if db.writeDelayNMeter != nil {
			db.writeDelayNMeter.Mark(writeDelayCounts[i%2] - writeDelayCounts[(i-1)%2])
		}
		if db.writeDelayMeter != nil {
			db.writeDelayNMeter.Mark(writeDelayTimes[i%2] - writeDelayTimes[(i-1)%2])
		}
		if db.compTimeMeter != nil {
			db.compTimeMeter.Mark(compactionTimes[i%2] - compactionTimes[(i-1)%2])
		}
		if db.compReadMeter != nil {
			db.compReadMeter.Mark(compactionReads[i%2] - compactionReads[(i-1)%2])
		}
		if db.compWriteMeter != nil {
			db.compWriteMeter.Mark(compactionWrites[i%2] - compactionWrites[(i-1)%2])
		}
		if db.diskSizeGauge != nil {
			db.diskSizeGauge.Update(int64(metrics.DiskSpaceUsage()))
		}

		db.memCompGauge.Update(metrics.Flush.Count)
		db.nonlevel0CompGauge.Update(metrics.Compact.Count)

		db.level0CompGauge.Update(0) // todo FIX ME
		db.seekCompGauge.Update(0)   // todo FIX ME

		// Sleep a bit, then repeat the stats collection
		select {
		case errc = <-db.quitChan:
			// Quit requesting, stop hammering the database
		case <-timer.C:
			timer.Reset(refresh)
			// Timeout, gather a new set of stats
		}
	}
	errc <- nil
}

// batch is a write-only leveldb batch that commits changes to its host database
// when Write is called. A batch cannot be used concurrently.
type batch struct {
	b    *pebble.Batch
	size int
}

// Put inserts the given value into the batch for later committing.
func (b *batch) Put(key, value []byte) error {
	b.b.Set(key, value, nil)
	b.size += len(value)
	return nil
}

// Delete inserts the a key removal into the batch for later committing.
func (b *batch) Delete(key []byte) error {
	b.b.Delete(key, nil)
	b.size++
	return nil
}

// ValueSize retrieves the amount of data queued up for writing.
func (b *batch) ValueSize() int {
	return b.size
}

// Write flushes any accumulated data to disk.
func (b *batch) Write() error {
	return b.b.Commit(pebble.NoSync)
}

// Reset resets the batch for reuse.
func (b *batch) Reset() {
	b.b.Reset()
	b.size = 0
}

// Replay replays the batch contents.
func (b *batch) Replay(w ethdb.KeyValueWriter) error {
	reader := b.b.Reader()
	for {
		kind, k, v, ok := reader.Next()
		if !ok {
			break
		}
		// I have no idea whether the iterated key and value
		// are safe to use, deep copy them temporarily.
		if kind == pebble.InternalKeyKindSet {
			w.Put(common.CopyBytes(k), common.CopyBytes(v))
		} else if kind == pebble.InternalKeyKindDelete {
			w.Delete(common.CopyBytes(k))
		} else {
			return errors.New("invalid operation") // todo FIX IT
		}
	}
	return nil
}

// pebbleIterator is a wrapper of underlying iterator in storage engine.
// The purpose of this structure is to implement the missing APIs.
type pebbleIterator struct {
	iter  *pebble.Iterator
	moved bool
}

// Next moves the iterator to the next key/value pair. It returns whether the
// iterator is exhausted.
func (iter *pebbleIterator) Next() bool {
	if iter.moved {
		iter.moved = false
		return iter.iter.Valid()
	}
	return iter.iter.Next()
}

// Error returns any accumulated error. Exhausting all the key/value pairs
// is not considered to be an error.
func (iter *pebbleIterator) Error() error {
	return iter.iter.Error()
}

// Key returns the key of the current key/value pair, or nil if done. The caller
// should not modify the contents of the returned slice, and its contents may
// change on the next call to Next.
func (iter *pebbleIterator) Key() []byte {
	return iter.iter.Key()
}

// Value returns the value of the current key/value pair, or nil if done. The
// caller should not modify the contents of the returned slice, and its contents
// may change on the next call to Next.
func (iter *pebbleIterator) Value() []byte {
	return iter.iter.Value()
}

// Release releases associated resources. Release should always succeed and can
// be called multiple times without causing error.
func (iter *pebbleIterator) Release() { iter.iter.Close() }

// bytesPrefixRange returns key range that satisfy
// - the given prefix, and
// - the given seek position
func bytesPrefixRange(prefix, start []byte) *util.Range {
	r := util.BytesPrefix(prefix)
	r.Start = append(r.Start, start...)
	return r
}
