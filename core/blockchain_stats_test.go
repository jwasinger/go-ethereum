// Copyright 2026 The go-ethereum Authors
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

package core

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
)

// TestStateCountsAdd locks down the count merge primitive used to aggregate
// per-tx, pre-tx and post-tx counters in the BAL parallel path.
func TestStateCountsAdd(t *testing.T) {
	a := state.StateCounts{
		AccountLoaded:   1,
		AccountUpdated:  2,
		AccountDeleted:  3,
		StorageLoaded:   4,
		StorageUpdated:  5,
		StorageDeleted:  6,
		CodeLoaded:      7,
		CodeLoadBytes:   8,
		CodeUpdated:     9,
		CodeUpdateBytes: 10,
	}
	b := state.StateCounts{
		AccountLoaded:   100,
		AccountUpdated:  200,
		AccountDeleted:  300,
		StorageLoaded:   400,
		StorageUpdated:  500,
		StorageDeleted:  600,
		CodeLoaded:      700,
		CodeLoadBytes:   800,
		CodeUpdated:     900,
		CodeUpdateBytes: 1000,
	}
	a.Add(&b)
	want := state.StateCounts{
		AccountLoaded:   101,
		AccountUpdated:  202,
		AccountDeleted:  303,
		StorageLoaded:   404,
		StorageUpdated:  505,
		StorageDeleted:  606,
		CodeLoaded:      707,
		CodeLoadBytes:   808,
		CodeUpdated:     909,
		CodeUpdateBytes: 1010,
	}
	if a != want {
		t.Fatalf("Add mismatch: got %+v, want %+v", a, want)
	}
}

// fixtureBlock builds a minimal *types.Block usable as the slow-block log
// subject. Only the header fields read by buildSlowBlockLog matter
// (Number, GasUsed, plus Transactions count via Body).
func fixtureBlock(number uint64, gasUsed uint64) *types.Block {
	header := &types.Header{
		Number:  new(big.Int).SetUint64(number),
		GasUsed: gasUsed,
	}
	return types.NewBlockWithHeader(header)
}

// TestBuildSlowBlockLog_NonBALShape ensures non-BAL output doesn't include
// the optional `bal` block (omitempty contract).
func TestBuildSlowBlockLog_NonBALShape(t *testing.T) {
	stats := &ExecuteStats{
		AccountReads:  3 * time.Millisecond,
		StorageReads:  4 * time.Millisecond,
		AccountHashes: 5 * time.Millisecond,
		Execution:     7 * time.Millisecond,
		TotalTime:     20 * time.Millisecond,
		MgasPerSecond: 12.5,
		StateCounts: state.StateCounts{
			AccountLoaded:  9,
			StorageLoaded:  21,
			StorageUpdated: 42,
		},
		// balTransitionStats deliberately nil — non-BAL block.
	}
	block := fixtureBlock(1234, 21000)
	logEntry := buildSlowBlockLog(stats, block)

	if logEntry.BAL != nil {
		t.Fatalf("non-BAL log unexpectedly has bal extension: %+v", logEntry.BAL)
	}
	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	wantKeys := map[string]bool{
		"level": true, "msg": true, "block": true, "timing": true,
		"throughput": true, "state_reads": true, "state_writes": true, "cache": true,
	}
	for k := range decoded {
		if !wantKeys[k] {
			t.Errorf("unexpected top-level key %q in non-BAL output (full JSON: %s)", k, string(jsonBytes))
		}
		delete(wantKeys, k)
	}
	if len(wantKeys) != 0 {
		t.Errorf("missing top-level keys: %v", wantKeys)
	}
	// Spot-check a count field that exercises the int64→int conversion in
	// state_writes.storage_slots (StorageUpdated is int64 on StateCounts).
	writes := decoded["state_writes"].(map[string]any)
	if got := writes["storage_slots"].(float64); got != 42 {
		t.Errorf("storage_slots: got %v, want 42", got)
	}
}

// TestBuildSlowBlockLog_BALShape ensures BAL output includes the bal extension
// with all expected sub-keys.
func TestBuildSlowBlockLog_BALShape(t *testing.T) {
	balMetrics := &state.BALStateTransitionMetrics{
		StatePrefetch:   1 * time.Millisecond,
		AccountUpdate:   2 * time.Millisecond,
		StateUpdate:     3 * time.Millisecond,
		StateHash:       4 * time.Millisecond,
		AccountCommits:  5 * time.Millisecond,
		StorageCommits:  6 * time.Millisecond,
		TrieDBCommits:   7 * time.Millisecond,
		SnapshotCommits: 8 * time.Millisecond,
	}
	stats := &ExecuteStats{
		AccountReads:       11 * time.Millisecond,
		StorageReads:       22 * time.Millisecond,
		CodeReads:          3 * time.Millisecond,
		Execution:          15 * time.Millisecond,
		TotalTime:          20 * time.Millisecond,
		MgasPerSecond:      30.0,
		ExecWall:           15 * time.Millisecond,
		PostProcess:        2 * time.Millisecond,
		Prefetch:           1 * time.Millisecond,
		balTransitionStats: balMetrics,
		StateCounts: state.StateCounts{
			AccountUpdated:  3,
			AccountDeleted:  1,
			CodeUpdated:     2,
			CodeUpdateBytes: 1024,
		},
		StateReadCacheStats: state.ReaderStats{
			StateStats: state.StateReaderStats{
				AccountCacheHit:  10,
				AccountCacheMiss: 5,
				StorageCacheHit:  20,
				StorageCacheMiss: 8,
			},
		},
	}
	block := fixtureBlock(7, 100000)
	logEntry := buildSlowBlockLog(stats, block)

	if logEntry.BAL == nil {
		t.Fatal("BAL log missing the bal extension")
	}
	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	bal, ok := decoded["bal"].(map[string]any)
	if !ok {
		t.Fatalf("bal key not present in JSON or wrong type; full JSON: %s", string(jsonBytes))
	}
	wantSubkeys := []string{
		"exec_wall_ms", "post_process_ms", "prefetch_ms",
		"state_prefetch_ms", "account_update_ms", "state_update_ms", "state_hash_ms",
		"account_commit_ms", "storage_commit_ms", "triedb_commit_ms", "snapshot_commit_ms",
	}
	for _, k := range wantSubkeys {
		if _, present := bal[k]; !present {
			t.Errorf("bal extension missing key %q", k)
		}
	}
	// Spot-check a value: exec_wall_ms should be 15.0 (15ms).
	if got := bal["exec_wall_ms"].(float64); got != 15.0 {
		t.Errorf("exec_wall_ms: got %v, want 15.0", got)
	}

	// state_read_ms = AccountReads + StorageReads + CodeReads = 11 + 22 + 3 = 36 ms
	timing := decoded["timing"].(map[string]any)
	if got := timing["state_read_ms"].(float64); got != 36 {
		t.Errorf("timing.state_read_ms: got %v, want 36", got)
	}

	writes := decoded["state_writes"].(map[string]any)
	if got := writes["accounts"].(float64); got != 3 {
		t.Errorf("state_writes.accounts: got %v, want 3", got)
	}
	if got := writes["accounts_deleted"].(float64); got != 1 {
		t.Errorf("state_writes.accounts_deleted: got %v, want 1", got)
	}
	if got := writes["code"].(float64); got != 2 {
		t.Errorf("state_writes.code: got %v, want 2", got)
	}
	if got := writes["code_bytes"].(float64); got != 1024 {
		t.Errorf("state_writes.code_bytes: got %v, want 1024", got)
	}

	cache := decoded["cache"].(map[string]any)
	acct := cache["account"].(map[string]any)
	if got := acct["hits"].(float64); got != 10 {
		t.Errorf("cache.account.hits: got %v, want 10", got)
	}
	if got := acct["misses"].(float64); got != 5 {
		t.Errorf("cache.account.misses: got %v, want 5", got)
	}
	storage := cache["storage"].(map[string]any)
	if got := storage["hits"].(float64); got != 20 {
		t.Errorf("cache.storage.hits: got %v, want 20", got)
	}
	if got := storage["misses"].(float64); got != 8 {
		t.Errorf("cache.storage.misses: got %v, want 8", got)
	}
}

// TestBuildSlowBlockLog_EmptyBlock ensures the helper handles a zero-tx,
// zero-counts block without panic and produces marshalable JSON.
func TestBuildSlowBlockLog_EmptyBlock(t *testing.T) {
	stats := &ExecuteStats{}
	block := fixtureBlock(0, 0)
	logEntry := buildSlowBlockLog(stats, block)
	if _, err := json.Marshal(logEntry); err != nil {
		t.Fatalf("empty block marshal failed: %v", err)
	}
	if logEntry.BAL != nil {
		t.Errorf("empty block should not have bal extension")
	}
}
