// Copyright 2025 The go-ethereum Authors
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

package miner

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/internal/telemetry"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// defaultParallelBatchSize is the number of most-profitable candidate
// transactions considered in each speculative round when ParallelBatchSize is
// left unset.
const defaultParallelBatchSize = 64

var debugParallel = false

// useParallelBuild reports whether the conflict-aware parallel builder should
// be used for the given environment. The builder relies on the per-transaction
// block access list (and the coinbase-read flag) that is only constructed once
// the Amsterdam fork is active, so it falls back to the sequential builder
// otherwise.
func (miner *Miner) useParallelBuild(env *environment) bool {
	if !miner.config.ParallelBuild {
		return false
	}
	return miner.chainConfig.IsAmsterdam(env.header.Number, env.header.Time)
}

// parallelBatchSize returns the configured batch size N, applying the default
// when unset.
func (miner *Miner) parallelBatchSize() int {
	if miner.config.ParallelBatchSize > 0 {
		return miner.config.ParallelBatchSize
	}
	return defaultParallelBatchSize
}

// candidate is a single transaction under consideration for inclusion. It holds
// the lazily-resolved transaction along with its effective miner tip, used to
// rank candidates by profitability.
type candidate struct {
	ltx  *txpool.LazyTransaction
	from common.Address
	tip  *uint256.Int
}

// specResult is the outcome of speculatively executing a single candidate
// against a copy of the pending state. Everything needed to commit the
// transaction without re-executing it is captured here: the resulting block
// access list (whose recorded post-state is applied directly), the receipt,
// and the gas the transaction consumed in each EIP-8037 dimension.
type specResult struct {
	cand    *candidate
	tx      *types.Transaction
	specIdx uint32 // block-access index used during speculation
	result  *bal.FinaliseResult
	receipt *types.Receipt
	// Per-transaction gas, read back from the throwaway speculative gas pool
	// (which started empty, so its cumulatives are this tx's contribution).
	regularGas uint64
	stateGas   uint64
	gasUsed    uint64
	err        error
}

// fillTransactionsParallel packs transactions into the block using the
// conflict-aware parallel strategy:
//
//  1. Take the N most profitable currently-eligible transactions (the lowest
//     pending nonce of each sender), ordered by effective tip.
//  2. Speculatively execute all N in parallel against the current pending
//     state, collecting each transaction's bal.FinaliseResult.
//  3. Greedily pack the most profitable subset whose state accesses do not
//     conflict (see txsConflict). A transaction that reads the coinbase
//     balance during EVM execution cannot be parallelised and is only packed
//     on its own.
//  4. Commit the packed subset to the block. Transactions that conflicted are
//     left in place and reconsidered in the next round.
//
// The loop repeats until the block is full, the candidate set is exhausted, or
// the build is interrupted.
//
// The plain and blob candidate maps are reowned by this function and must not
// be used by the caller afterwards.
func (miner *Miner) fillTransactionsParallel(ctx context.Context, interrupt *atomic.Int32, env *environment, plainTxs, blobTxs map[common.Address][]*txpool.LazyTransaction) (err error) {
	ctx, _, spanEnd := telemetry.StartSpan(ctx, "miner.fillTransactionsParallel")
	defer spanEnd(&err)

	// Merge the per-account, nonce-ordered plain and blob lists into a single
	// candidate set keyed by sender. Each account contributes at most one
	// eligible transaction (its head) per round, which preserves intra-account
	// nonce ordering for free.
	pending := mergePendingByAccount(plainTxs, blobTxs)
	if len(pending) == 0 {
		return nil
	}
	// cursor[addr] points at the next un-committed transaction for the account.
	cursor := make(map[common.Address]int, len(pending))

	var baseFee *uint256.Int
	if env.header.BaseFee != nil {
		baseFee = uint256.MustFromBig(env.header.BaseFee)
	}
	// The block context is independent of the executing state and is reused for
	// every speculative execution.
	blockCtx := core.NewEVMBlockContext(env.header, miner.chain, &env.coinbase)
	batchSize := miner.parallelBatchSize()

	for {
		// Honour interruption signals (timeout / new head / resubmit).
		if interrupt != nil {
			if signal := interrupt.Load(); signal != commitInterruptNone {
				return signalToErr(signal)
			}
		}
		// If we are out of gas for even the cheapest transaction, the block is
		// full.
		if env.gasPool.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "have", env.gasPool, "want", params.TxGas)
			break
		}

		// TODO: check that the tx fits in the block
		//  or check at the time of packing txs from a batch into the block.

		// Gather the eligible head transaction of every account, drop any that
		// can no longer pay the base fee, and rank by effective tip.
		eligible, dropped := collectEligible(pending, cursor, baseFee)
		if len(eligible) == 0 {
			break
		}
		if len(eligible) > batchSize {
			eligible = eligible[:batchSize]
		}

		// Speculatively execute the selected candidates in parallel.
		results := miner.speculateBatch(env, blockCtx, eligible)

		// Pick the non-conflicting, most-profitable subset and remember which
		// accounts produced stale or genuinely invalid transactions.
		selected, shiftAccts, dropAccts := selectNonConflicting(results, env.coinbase)
		for _, addr := range shiftAccts {
			dropped = true
			cursor[addr]++ // skip the stale head, keep the rest
		}
		for _, addr := range dropAccts {
			dropped = true
			cursor[addr] = len(pending[addr]) // drop the account's remaining txs
		}

		// Commit the packed subset in profitability order by applying each
		// transaction's recorded post-state from its access list — no
		// re-execution. The coinbase fee is accumulated across the round, so
		// capture the round's starting coinbase balance (identical to what the
		// speculative state copies observed).
		roundStartCoinbase := env.state.GetBalance(env.coinbase).Clone()
		committed := false
		for _, res := range selected {
			ok, dropAcct := miner.commitFromResult(env, res, roundStartCoinbase)
			switch {
			case ok:
				cursor[res.cand.from]++
				committed = true
			case dropAcct:
				// Did not fit the block's gas/blob/size budget; exclude the
				// sender from the rest of this build (it stays in the pool).
				cursor[res.cand.from] = len(pending[res.cand.from])
				dropped = true
			}
		}

		// Guard against a stuck loop: if a full round neither committed nor
		// dropped anything (e.g. every candidate conflicted yet none could be
		// committed because the block is effectively full), stop.
		if !committed && !dropped {
			break
		}
	}
	return nil
}

// mergePendingByAccount combines the nonce-ordered plain and blob transaction
// lists into a single per-account list. The common case (an account appearing
// in only one of the two maps) is a cheap reference copy; the rare case of an
// account with both plain and blob pending transactions resolves nonces to
// merge the two lists in order.
func mergePendingByAccount(plainTxs, blobTxs map[common.Address][]*txpool.LazyTransaction) map[common.Address][]*txpool.LazyTransaction {
	merged := make(map[common.Address][]*txpool.LazyTransaction, len(plainTxs)+len(blobTxs))
	for addr, txs := range plainTxs {
		if len(txs) > 0 {
			merged[addr] = txs
		}
	}
	for addr, btxs := range blobTxs {
		if len(btxs) == 0 {
			continue
		}
		existing, ok := merged[addr]
		if !ok {
			merged[addr] = btxs
			continue
		}
		merged[addr] = mergeByNonce(existing, btxs)
	}
	return merged
}

// mergeByNonce merges two nonce-ordered transaction lists into one. Resolving
// is only triggered here, on the rare path where a single sender has both
// plain and blob transactions pending.
func mergeByNonce(a, b []*txpool.LazyTransaction) []*txpool.LazyTransaction {
	out := make([]*txpool.LazyTransaction, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ta, tb := a[i].Resolve(), b[j].Resolve()
		// Treat an unresolvable (evicted) transaction as sorting last so the
		// other list makes progress.
		switch {
		case ta == nil:
			i++
		case tb == nil:
			j++
		case ta.Nonce() <= tb.Nonce():
			out = append(out, a[i])
			i++
		default:
			out = append(out, b[j])
			j++
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}

// collectEligible returns the head (lowest pending nonce) transaction of each
// account, ranked by effective miner tip in descending order. Accounts whose
// head can no longer pay the base fee have their entire remaining queue
// dropped (matching the sequential builder, which discards a sender once one
// of its transactions becomes unpayable). The boolean return reports whether
// any account was dropped, which the caller uses for loop-progress accounting.
func collectEligible(pending map[common.Address][]*txpool.LazyTransaction, cursor map[common.Address]int, baseFee *uint256.Int) ([]*candidate, bool) {
	dropped := false
	eligible := make([]*candidate, 0, len(pending))
	for addr, txs := range pending {
		idx := cursor[addr]
		if idx >= len(txs) {
			continue
		}
		ltx := txs[idx]
		tip, ok := effectiveTip(ltx, baseFee)
		if !ok {
			// Unpayable at the current base fee; drop this sender entirely.
			cursor[addr] = len(txs)
			dropped = true
			continue
		}
		eligible = append(eligible, &candidate{ltx: ltx, from: addr, tip: tip})
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		c := eligible[i].tip.Cmp(eligible[j].tip)
		if c != 0 {
			return c > 0 // higher tip first
		}
		// Tie-break on first-seen time for determinism, mirroring the heap used
		// by the sequential builder.
		return eligible[i].ltx.Time.Before(eligible[j].ltx.Time)
	})
	return eligible, dropped
}

// effectiveTip computes the effective miner tip per gas for a candidate, using
// the same rule as the sequential ordering: min(GasTipCap, GasFeeCap-baseFee).
// It reports false when the fee cap cannot cover the base fee.
func effectiveTip(ltx *txpool.LazyTransaction, baseFee *uint256.Int) (*uint256.Int, bool) {
	tip := new(uint256.Int).Set(ltx.GasTipCap)
	if baseFee != nil {
		if ltx.GasFeeCap.Cmp(baseFee) < 0 {
			return nil, false
		}
		tip = new(uint256.Int).Sub(ltx.GasFeeCap, baseFee)
		if tip.Gt(ltx.GasTipCap) {
			tip.Set(ltx.GasTipCap)
		}
	}
	return tip, true
}

// speculateBatch resolves and speculatively executes every candidate in
// parallel, each against an isolated copy of the pending state. The state
// copies are made serially on the calling goroutine (the only accessor of the
// live state) and the EVM executions run concurrently on those copies, so the
// live environment is never touched from multiple goroutines.
func (miner *Miner) speculateBatch(env *environment, blockCtx vm.BlockContext, candidates []*candidate) []*specResult {
	results := make([]*specResult, len(candidates))
	states := make([]*state.StateDB, len(candidates))

	// Build a single read-only access-list reader over everything committed so
	// far this block. env.bal already records the pre-execution system calls
	// (at block-access index 0) and every committed transaction (at its real
	// index), so a reader queried at the next index serves the post-state of
	// the committed prefix. Every candidate in the round speculates against
	// that same prefix, so the reader (and the pristine base it overlays) is
	// shared across the batch and built once. This mirrors the import path,
	// which shares one prepared access list across all per-transaction readers
	// (core/parallel_state_processor.go).
	committed := bal.NewAccessListReader(*env.bal.ToEncodingObj())
	readIdx := env.tcount + 1
	baseReader := env.baseState.Reader()

	// Phase 1 (serial): resolve transactions and layer a fresh reader over the
	// pristine base. Unlike a copy of the live, accumulating state, the cost of
	// this does not grow with the number of already-committed transactions:
	// their effects are served from the access list above rather than baked
	// into an ever-larger statedb copy.
	for i, cand := range candidates {
		tx := cand.ltx.Resolve()
		results[i] = &specResult{cand: cand, tx: tx}
		if tx == nil {
			// Evicted from the pool between selection and resolution.
			results[i].err = core.ErrNonceTooLow
			continue
		}
		// WithReader clones baseState and swaps in the reader. Because baseState
		// carries no loaded objects, the reader is authoritative: reads of state
		// touched by the committed prefix resolve through the access list, and
		// everything else falls through to the parent state.
		states[i] = env.baseState.WithReader(state.NewReaderWithAccessList(baseReader, committed, readIdx))
	}

	// Phase 2 (parallel): execute each candidate on its own state copy.
	workers := runtime.NumCPU()
	if workers > len(candidates) {
		workers = len(candidates)
	}
	if workers < 1 {
		workers = 1
	}
	var (
		wg   sync.WaitGroup
		next atomic.Int64
	)
	next.Store(0)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(candidates) {
					return
				}
				if results[i].tx == nil {
					continue
				}
				// Use the batch position as the (temporary) block-access index;
				// it is remapped to the real position at commit time.
				results[i].specIdx = uint32(i + 1)
				miner.speculateOne(env, blockCtx, states[i], results[i])
			}
		}()
	}
	wg.Wait()
	return results
}

// speculateOne executes a single transaction against a copy of the pending
// state, recording into res everything needed to later commit the transaction
// without re-executing it: the bal.FinaliseResult (access list with recorded
// post-state, plus the coinbase-read flag), the receipt, and the per-dimension
// gas the transaction consumed. The state copy is discarded afterwards; only
// the recorded results are kept.
func (miner *Miner) speculateOne(env *environment, blockCtx vm.BlockContext, statedb *state.StateDB, res *specResult) {
	tx := res.tx
	// res.specIdx (the batch position) was assigned by the caller; it is
	// remapped to the real block position when the transaction is committed.
	statedb.SetTxContext(tx.Hash(), int(res.specIdx), res.specIdx)

	msg, err := core.TransactionToMessage(tx, env.signer, env.header.BaseFee)
	if err != nil {
		res.err = err
		return
	}
	evm := vm.NewEVM(blockCtx, statedb, miner.chainConfig, vm.Config{})
	defer evm.Release()

	// A fresh, full-block gas pool. Because it starts empty, after a single
	// transaction its cumulative regular/state/used counters hold exactly this
	// transaction's contribution, which is reused verbatim at commit time.
	gp := core.NewGasPool(env.header.GasLimit)
	receipt, accessList, err := core.ApplyTransactionWithEVM(msg, gp, statedb, env.header.Number, env.header.Hash(), env.header.Time, tx, evm)
	if err != nil {
		res.err = err
		return
	}
	res.result = bal.NewFinaliseResult(accessList, statedb.CoinbaseRead())
	res.receipt = receipt
	res.regularGas = gp.CumulativeRegular()
	res.stateGas = gp.CumulativeState()
	res.gasUsed = gp.CumulativeUsed()
}

// selectNonConflicting walks the speculative results in profitability order and
// greedily selects a non-conflicting subset. It returns the chosen results (in
// commit order) along with the senders whose head should be advanced by one (a
// stale, already-included transaction) and the senders whose remaining queue
// should be dropped (a genuinely invalid transaction or a nonce gap).
// Transactions that merely conflicted with a higher-priority selection are
// neither selected nor advanced/dropped; they remain eligible for a later round.
func selectNonConflicting(results []*specResult, coinbase common.Address) (selected []*specResult, shiftAccts, dropAccts []common.Address) {
	acc := newFootprint()
	for _, res := range results {
		if res.err != nil {
			switch {
			case errors.Is(res.err, core.ErrNonceTooLow):
				// Stale (already-included) head; skip just this one.
				shiftAccts = append(shiftAccts, res.cand.from)
			default:
				// Invalid transaction or a nonce gap (nonce-too-high at the
				// head means the rest of the queue is unreachable). Drop the
				// sender's remaining queue, mirroring the sequential builder's
				// Pop on a failed transaction.
				dropAccts = append(dropAccts, res.cand.from)
			}
			continue
		}
		fp := footprintFor(res.result, coinbase)
		if res.result.CoinbaseRead() {
			// Reads the coinbase balance during execution: ordering-dependent,
			// so it cannot be parallelised. Only include it when it can stand
			// alone as the entire batch.
			if len(selected) == 0 {
				selected = append(selected, res)
			}
			// Whether or not it was taken, nothing else can join a batch that
			// contains (or would contain) a coinbase reader.
			break
		}
		if acc.conflicts(fp) {
			continue // leave for a later round
		}
		acc.merge(fp)
		selected = append(selected, res)
	}
	return selected, shiftAccts, dropAccts
}

// canInclude reports whether the given transaction still fits the block's gas,
// blob, and size budgets. It mirrors the inline checks of the sequential
// commitTransactions loop.
func (miner *Miner) canInclude(env *environment, tx *types.Transaction) bool {
	if env.gasPool.Gas() < tx.Gas() {
		return false
	}
	if !env.txFitsSize(tx) {
		return false
	}
	if tx.Type() == types.BlobTxType {
		maxBlobs := miner.maxBlobsPerBlock(env.header.Time)
		if env.blobs+len(tx.BlobHashes()) > maxBlobs {
			return false
		}
	}
	return true
}

// commitFromResult includes a speculatively-executed transaction in the block
// by applying its recorded post-state directly from the access list, without
// running the EVM a second time. This is sound because the packed subset is
// non-conflicting: each non-coinbase account or storage slot is touched by at
// most one transaction in the round, so the post-state value recorded against
// the round's starting state is also its value in the committed sequence.
//
// The coinbase is the sole exception — every transaction credits it the fee —
// so its balance is accumulated across the round rather than overwritten:
// roundStartCoinbase is the balance all of the round's speculative copies
// observed, and each transaction's net effect on the coinbase is the difference
// between its recorded coinbase balance and that starting value.
//
// It returns committed=true on success, or dropAccount=true when the
// transaction no longer fits the block's gas/blob/size budget (in which case
// the block is effectively full for that sender for the remainder of this
// build).
func (miner *Miner) commitFromResult(env *environment, res *specResult, roundStartCoinbase *uint256.Int) (committed bool, dropAccount bool) {
	tx := res.tx
	if !miner.canInclude(env, tx) {
		return false, true
	}
	// Charge the block gas pool using the per-dimension gas captured during
	// speculation (EIP-8037). A failure means the block cannot fit this tx.
	if err := env.gasPool.ChargeGasAmsterdam(res.regularGas, res.stateGas, res.gasUsed); err != nil {
		return false, true
	}
	al := res.result.AccessList()
	realIdx := uint32(env.tcount + 1)
	if debugParallel {
		naccts := 0
		if al != nil {
			naccts = len(al.Accounts)
		}
		println("commitFromResult tx", res.tx.Hash().Hex(), "alAccounts", naccts, "gasUsed", res.gasUsed, "specIdx", res.specIdx)
	}

	// Establish the transaction context so re-played logs are indexed against
	// the real block position.
	env.state.SetTxContext(tx.Hash(), env.tcount, realIdx)

	// 1. Apply every recorded mutation except the coinbase balance.
	applyAccessListState(env.state, al, env.coinbase)

	// 2. Fold this transaction's coinbase delta into the running balance.
	coinbaseBal := env.state.GetBalance(env.coinbase) // running cumulative
	newCoinbaseBal := coinbaseBal.Clone()
	if cur := recordedCoinbaseBalance(al, env.coinbase, res.specIdx); cur != nil {
		// delta = cur - roundStartCoinbase (signed), applied to the running
		// balance: newCoinbase = coinbaseBal + (cur - roundStartCoinbase).
		if cur.Cmp(roundStartCoinbase) >= 0 {
			newCoinbaseBal.Add(newCoinbaseBal, new(uint256.Int).Sub(cur, roundStartCoinbase))
		} else {
			newCoinbaseBal.Sub(newCoinbaseBal, new(uint256.Int).Sub(roundStartCoinbase, cur))
		}
		env.state.SetBalance(env.coinbase, newCoinbaseBal, tracing.BalanceIncreaseRewardTransactionFee)
	}

	// 3. Re-play the logs so they receive correct block/tx/log indices, then
	//    build the receipt from the speculative one with positional fields fixed.
	for _, lg := range res.receipt.Logs {
		env.state.AddLog(lg)
	}
	receipt := res.receipt
	receipt.CumulativeGasUsed = env.gasPool.CumulativeUsed()
	receipt.TransactionIndex = uint(env.tcount)
	receipt.Logs = env.state.GetLogs(tx.Hash(), env.header.Number.Uint64(), env.header.Hash(), env.header.Time)
	receipt.Bloom = types.CreateBloom(receipt)

	// 4. Fold the access list into the block-level list, remapping its index to
	//    the real block position and recording the cumulative (not isolated)
	//    coinbase balance.
	remapAccessListIndex(al, res.specIdx, realIdx)
	if cur := recordedCoinbaseBalance(al, env.coinbase, realIdx); cur != nil {
		al.BalanceChange(realIdx, env.coinbase, newCoinbaseBal)
	}
	env.bal.Merge(al)

	// 5. Block/header bookkeeping, mirroring commitTransaction / commitBlobTransaction.
	env.header.GasUsed = env.gasPool.Used()
	if tx.Type() == types.BlobTxType {
		sc := tx.BlobTxSidecar()
		txNoBlob := tx.WithoutBlobTxSidecar()
		env.txs = append(env.txs, txNoBlob)
		env.sidecars = append(env.sidecars, sc)
		env.blobs += len(sc.Blobs)
		*env.header.BlobGasUsed += receipt.BlobGasUsed
		env.size += txNoBlob.Size()
	} else {
		env.txs = append(env.txs, tx)
		env.size += tx.Size()
	}
	env.receipts = append(env.receipts, receipt)
	env.tcount++
	return true, false
}

// applyAccessListState writes every state mutation recorded in the access list
// to statedb: storage slots, nonces, code, and balances. The coinbase balance
// is skipped (handled separately, since it accumulates the fee from every
// transaction); all of the coinbase's other fields are still applied.
func applyAccessListState(statedb *state.StateDB, al *bal.ConstructionBlockAccessList, coinbase common.Address) {
	if al == nil {
		return
	}
	for addr, aa := range al.Accounts {
		for slot, byIdx := range aa.StorageWrites {
			for _, value := range byIdx {
				statedb.SetState(addr, slot, value)
			}
		}
		for _, nonce := range aa.NonceChanges {
			statedb.SetNonce(addr, nonce, tracing.NonceChangeUnspecified)
		}
		for _, code := range aa.CodeChange {
			statedb.SetCode(addr, code, tracing.CodeChangeUnspecified)
		}
		if addr == coinbase {
			continue // balance handled by the caller
		}
		for _, balance := range aa.BalanceChanges {
			statedb.SetBalance(addr, balance, tracing.BalanceChangeUnspecified)
		}
	}
}

// recordedCoinbaseBalance returns the coinbase balance recorded in the access
// list at the given index, or nil if the coinbase balance was not changed.
func recordedCoinbaseBalance(al *bal.ConstructionBlockAccessList, coinbase common.Address, idx uint32) *uint256.Int {
	if al == nil {
		return nil
	}
	aa, ok := al.Accounts[coinbase]
	if !ok {
		return nil
	}
	return aa.BalanceChanges[idx]
}

// remapAccessListIndex rewrites every per-transaction index key in the access
// list from `from` to `to`. A construction access list produced for a single
// transaction keys all of its entries under one index (the value passed to
// SetTxContext during speculation); committing it into the block requires that
// index to match the transaction's real block position.
func remapAccessListIndex(al *bal.ConstructionBlockAccessList, from, to uint32) {
	if al == nil || from == to {
		return
	}
	for _, aa := range al.Accounts {
		for _, byIdx := range aa.StorageWrites {
			if v, ok := byIdx[from]; ok {
				byIdx[to] = v
				delete(byIdx, from)
			}
		}
		if v, ok := aa.BalanceChanges[from]; ok {
			aa.BalanceChanges[to] = v
			delete(aa.BalanceChanges, from)
		}
		if v, ok := aa.NonceChanges[from]; ok {
			aa.NonceChanges[to] = v
			delete(aa.NonceChanges, from)
		}
		if v, ok := aa.CodeChange[from]; ok {
			aa.CodeChange[to] = v
			delete(aa.CodeChange, from)
		}
	}
}

// slotKey identifies a single (account, storage-slot) pair.
type slotKey struct {
	addr common.Address
	slot common.Hash
}

// footprint summarises the state a transaction touches, separated into account-
// level and storage-level reads and writes. It is the basis for conflict
// detection: two transactions conflict when one writes state the other reads or
// writes.
type footprint struct {
	writeAccounts map[common.Address]struct{} // balance / nonce / code mutated
	readAccounts  map[common.Address]struct{} // account otherwise touched
	writeSlots    map[slotKey]struct{}
	readSlots     map[slotKey]struct{}
}

func newFootprint() *footprint {
	return &footprint{
		writeAccounts: make(map[common.Address]struct{}),
		readAccounts:  make(map[common.Address]struct{}),
		writeSlots:    make(map[slotKey]struct{}),
		readSlots:     make(map[slotKey]struct{}),
	}
}

// footprintFor derives the read/write footprint of a transaction from its
// construction access list.
//
// The coinbase account is deliberately excluded: every transaction's fee
// payment mutates the coinbase balance, so including it would make all
// transactions mutually conflicting. The genuine ordering hazard around the
// coinbase — a transaction observing its balance mid-block — is handled
// separately via the FinaliseResult's coinbase-read flag.
//
// Note the access list records account reads coarsely (any account it loads,
// including for storage access), so account-level conflict detection is
// conservative: it may flag a conflict between an account-level write and an
// unrelated access to the same account. This never produces an incorrect block
// (it only forgoes some parallelism), which is the safe direction to err.
func footprintFor(res *bal.FinaliseResult, coinbase common.Address) *footprint {
	fp := newFootprint()
	al := res.AccessList()
	if al == nil {
		return fp
	}
	for addr, aa := range al.Accounts {
		if addr == coinbase {
			continue
		}
		// Any presence in the access list means the account was touched.
		fp.readAccounts[addr] = struct{}{}
		if len(aa.BalanceChanges) > 0 || len(aa.NonceChanges) > 0 || len(aa.CodeChange) > 0 {
			fp.writeAccounts[addr] = struct{}{}
		}
		for slot := range aa.StorageWrites {
			fp.writeSlots[slotKey{addr, slot}] = struct{}{}
		}
		for slot := range aa.StorageReads {
			fp.readSlots[slotKey{addr, slot}] = struct{}{}
		}
	}
	return fp
}

// conflicts reports whether other conflicts with the accumulated footprint:
// a conflict exists when either side writes a piece of state (an account or a
// storage slot) that the other side touches. Two transactions that only read
// the same state do not conflict.
func (fp *footprint) conflicts(other *footprint) bool {
	// Account-level: a write on one side against any touch on the other.
	for addr := range other.writeAccounts {
		if _, ok := fp.readAccounts[addr]; ok {
			return true
		}
		if _, ok := fp.writeAccounts[addr]; ok {
			return true
		}
	}
	for addr := range fp.writeAccounts {
		if _, ok := other.readAccounts[addr]; ok {
			return true
		}
	}
	// Storage-level: same write-against-touch rule per (account, slot).
	for sk := range other.writeSlots {
		if _, ok := fp.readSlots[sk]; ok {
			return true
		}
		if _, ok := fp.writeSlots[sk]; ok {
			return true
		}
	}
	for sk := range fp.writeSlots {
		if _, ok := other.readSlots[sk]; ok {
			return true
		}
	}
	return false
}

// merge folds other into the accumulated footprint.
func (fp *footprint) merge(other *footprint) {
	for addr := range other.writeAccounts {
		fp.writeAccounts[addr] = struct{}{}
	}
	for addr := range other.readAccounts {
		fp.readAccounts[addr] = struct{}{}
	}
	for sk := range other.writeSlots {
		fp.writeSlots[sk] = struct{}{}
	}
	for sk := range other.readSlots {
		fp.readSlots[sk] = struct{}{}
	}
}
