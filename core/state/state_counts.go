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

package state

import "time"

// ReadDurations groups the {Account, Storage, Code} state-read times that are
// aggregated across pre-tx, per-tx and post-tx statedbs in the BAL parallel
// path. Sum-of-CPU-time, not wall-clock.
type ReadDurations struct {
	Account time.Duration
	Storage time.Duration
	Code    time.Duration
}

// Add merges other into r.
func (r *ReadDurations) Add(other ReadDurations) {
	r.Account += other.Account
	r.Storage += other.Storage
	r.Code += other.Code
}

// StateCounts holds count-only statistics gathered during a block's state
// transition. Plain-int snapshot type, safe to copy through channels.
// Atomic counters on StateDB are converted at the snapshot boundary in
// SnapshotCounts. Read durations live in ReadDurations (separate type).
type StateCounts struct {
	AccountLoaded   int   // accounts retrieved from the database during the state transition
	AccountUpdated  int   // accounts updated during the state transition
	AccountDeleted  int   // accounts deleted during the state transition
	StorageLoaded   int   // storage slots retrieved from the database during the state transition
	StorageUpdated  int64 // storage slots updated (snapshotted from atomic on StateDB)
	StorageDeleted  int64 // storage slots deleted (snapshotted from atomic on StateDB)
	CodeLoaded      int   // contract code reads
	CodeLoadBytes   int   // total bytes of resolved code
	CodeUpdated     int   // code writes (CREATE/CREATE2/EIP-7702)
	CodeUpdateBytes int   // total bytes of persisted code written
}

// Add merges other into c. Plain integer addition — no atomics here, since
// StateCounts is the snapshot type. The receiver is the only mutated party;
// other is taken by value (the struct is small and value semantics matches
// the snapshot thesis stated above).
func (c *StateCounts) Add(other StateCounts) {
	c.AccountLoaded += other.AccountLoaded
	c.AccountUpdated += other.AccountUpdated
	c.AccountDeleted += other.AccountDeleted
	c.StorageLoaded += other.StorageLoaded
	c.StorageUpdated += other.StorageUpdated
	c.StorageDeleted += other.StorageDeleted
	c.CodeLoaded += other.CodeLoaded
	c.CodeLoadBytes += other.CodeLoadBytes
	c.CodeUpdated += other.CodeUpdated
	c.CodeUpdateBytes += other.CodeUpdateBytes
}
