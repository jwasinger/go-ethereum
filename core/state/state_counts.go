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

// StateCounts holds count-only statistics gathered during a block's state
// transition. It is the snapshot/aggregation type: all fields are plain ints,
// safe to copy and pass by value through channels and struct fields.
//
// StateDB still uses atomic counters internally (for concurrent worker
// updates); the conversion to plain ints happens at the snapshot boundary
// in (*StateDB).SnapshotCounts. This separation keeps the live atomics
// scoped to the mutation surface and lets the rest of the pipeline use
// vet-clean value semantics.
//
// Only counts live here — time.Duration fields (AccountReads, StorageReads,
// etc.) stay on StateDB directly, since their parallel-execution semantics
// don't fit the simple Add merge pattern.
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
