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
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// amsterdamConfig returns a chain config with every fork (including Amsterdam,
// which activates the block-access-list machinery this builder relies on)
// active from genesis.
func amsterdamConfig() *params.ChainConfig {
	cfg := *params.MergedTestChainConfig
	t0 := uint64(0)
	cfg.AmsterdamTime = &t0
	sched := *cfg.BlobScheduleConfig
	sched.Amsterdam = params.DefaultOsakaBlobConfig // reuse Osaka schedule for the test
	cfg.BlobScheduleConfig = &sched
	return &cfg
}

// buildComparableBlock builds one block from a fresh chain seeded with the given
// signed transactions, using the parallel (BAL-application) builder when
// parallel is true and the sequential (re-execution) builder otherwise.
func buildComparableBlock(t *testing.T, cfg *params.ChainConfig, txs []*types.Transaction, alloc types.GenesisAlloc, parallel bool) *engine.ExecutableData {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	engine := beacon.New(ethash.NewFaker())
	gspec := &core.Genesis{
		Config:  cfg,
		Alloc:   alloc,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	chain, err := core.NewBlockChain(db, gspec, engine, &core.BlockChainConfig{ArchiveMode: true})
	if err != nil {
		t.Fatalf("NewBlockChain: %v", err)
	}
	defer chain.Stop()

	pool := legacypool.New(testTxPoolConfig, chain)
	pl, err := txpool.New(testTxPoolConfig.PriceLimit, chain, []txpool.SubPool{pool})
	if err != nil {
		t.Fatalf("txpool.New: %v", err)
	}
	defer pl.Close()

	cfgCopy := testConfig
	cfgCopy.ParallelBuild = parallel
	cfgCopy.GasCeil = 30_000_000
	backend := &testWorkerBackend{db: db, chain: chain, txPool: pl, genesis: gspec}
	miner := New(backend, cfgCopy, engine)

	if errs := pl.Add(txs, true); len(errs) > 0 {
		for _, e := range errs {
			if e != nil {
				t.Fatalf("txpool add: %v", e)
			}
		}
	}
	// Give the pool a moment to surface the transactions as pending.
	for i := 0; i < 100; i++ {
		p, _ := pl.Pending(txpool.PendingFilter{})
		if len(p) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	slot := uint64(1)
	parent := chain.CurrentBlock()
	args := &BuildPayloadArgs{
		Parent:       parent.Hash(),
		Timestamp:    parent.Time + 12,
		Random:       common.Hash{0x1},
		FeeRecipient: common.HexToAddress("0xc01b"),
		SlotNum:      &slot,
	}
	payload, err := miner.buildPayload(context.Background(), args, false)
	if err != nil {
		t.Fatalf("buildPayload (parallel=%v): %v", parallel, err)
	}
	env := payload.ResolveFull()
	if env == nil {
		t.Fatalf("nil payload envelope (parallel=%v)", parallel)
	}
	return env.ExecutionPayload
}

// TestParallelBuildMatchesSequential verifies that committing the packed,
// non-conflicting transactions by applying their recorded block-access-list
// post-state (instead of re-executing them) yields a block with the same state
// root, receipts root, gas usage, and transaction set as the sequential
// re-execution builder.
func TestParallelBuildMatchesSequential(t *testing.T) {
	cfg := amsterdamConfig()
	signer := types.LatestSigner(cfg)

	// A set of senders making disjoint (non-conflicting) value transfers, each
	// with a distinct, descending tip so both builders order them identically.
	const n = 6
	keys := make([]*ecdsa.PrivateKey, n)
	alloc := types.GenesisAlloc{
		params.BeaconRootsAddress:        {Nonce: 1, Code: params.BeaconRootsCode, Balance: common.Big0},
		params.HistoryStorageAddress:     {Nonce: 1, Code: params.HistoryStorageCode, Balance: common.Big0},
		params.WithdrawalQueueAddress:    {Nonce: 1, Code: params.WithdrawalQueueCode, Balance: common.Big0},
		params.ConsolidationQueueAddress: {Nonce: 1, Code: params.ConsolidationQueueCode, Balance: common.Big0},
	}
	var txs []*types.Transaction
	for i := 0; i < n; i++ {
		k, _ := crypto.GenerateKey()
		keys[i] = k
		addr := crypto.PubkeyToAddress(k.PublicKey)
		alloc[addr] = types.Account{Balance: new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))}

		recipient := common.BytesToAddress([]byte{0xde, 0xad, byte(i)})
		tip := big.NewInt(int64((n - i) * params.GWei)) // descending tips
		tx := types.MustSignNewTx(k, signer, &types.DynamicFeeTx{
			ChainID:   cfg.ChainID,
			Nonce:     0,
			To:        &recipient,
			Value:     big.NewInt(1000),
			Gas:       params.TxGas,
			GasTipCap: tip,
			GasFeeCap: new(big.Int).Add(big.NewInt(params.InitialBaseFee), tip),
		})
		txs = append(txs, tx)
	}

	seq := buildComparableBlock(t, cfg, txs, alloc, false)
	par := buildComparableBlock(t, cfg, txs, alloc, true)

	if len(seq.Transactions) != n {
		t.Fatalf("sequential build included %d txs, want %d", len(seq.Transactions), n)
	}
	if len(par.Transactions) != len(seq.Transactions) {
		t.Fatalf("tx count mismatch: parallel %d, sequential %d", len(par.Transactions), len(seq.Transactions))
	}
	for i := range seq.Transactions {
		if string(par.Transactions[i]) != string(seq.Transactions[i]) {
			t.Fatalf("tx %d differs between builders (ordering or content)", i)
		}
	}
	if par.GasUsed != seq.GasUsed {
		t.Fatalf("gas used mismatch: parallel %d, sequential %d", par.GasUsed, seq.GasUsed)
	}
	if par.StateRoot != seq.StateRoot {
		t.Fatalf("state root mismatch:\n parallel  %s\n sequential %s", par.StateRoot.Hex(), seq.StateRoot.Hex())
	}
	if par.ReceiptsRoot != seq.ReceiptsRoot {
		t.Fatalf("receipts root mismatch:\n parallel  %s\n sequential %s", par.ReceiptsRoot.Hex(), seq.ReceiptsRoot.Hex())
	}
}

// TestParallelBuildConflictsMatchState exercises the multi-round path: a set of
// transactions that conflict (several senders paying the same recipient) plus a
// same-sender nonce chain. Conflicting transactions are deferred to later
// rounds rather than dropped, and a sender's second nonce can only be committed
// after its first, so the parallel builder commits across several rounds and in
// a different order than the sequential builder. The resulting block therefore
// need not be byte-identical, but because these are plain value transfers the
// post-state is order-independent: the final state root, total gas, and number
// of included transactions must match.
func TestParallelBuildConflictsMatchState(t *testing.T) {
	cfg := amsterdamConfig()
	signer := types.LatestSigner(cfg)

	alloc := types.GenesisAlloc{
		params.BeaconRootsAddress:        {Nonce: 1, Code: params.BeaconRootsCode, Balance: common.Big0},
		params.HistoryStorageAddress:     {Nonce: 1, Code: params.HistoryStorageCode, Balance: common.Big0},
		params.WithdrawalQueueAddress:    {Nonce: 1, Code: params.WithdrawalQueueCode, Balance: common.Big0},
		params.ConsolidationQueueAddress: {Nonce: 1, Code: params.ConsolidationQueueCode, Balance: common.Big0},
	}
	mkSender := func() (*ecdsa.PrivateKey, common.Address) {
		k, _ := crypto.GenerateKey()
		a := crypto.PubkeyToAddress(k.PublicKey)
		alloc[a] = types.Account{Balance: new(big.Int).Mul(big.NewInt(10), big.NewInt(params.Ether))}
		return k, a
	}
	tx := func(k *ecdsa.PrivateKey, nonce uint64, to common.Address, tipGwei int64) *types.Transaction {
		tip := big.NewInt(tipGwei * params.GWei)
		return types.MustSignNewTx(k, signer, &types.DynamicFeeTx{
			ChainID: cfg.ChainID, Nonce: nonce, To: &to, Value: big.NewInt(1000),
			Gas: params.TxGas, GasTipCap: tip, GasFeeCap: new(big.Int).Add(big.NewInt(params.InitialBaseFee), tip),
		})
	}

	shared := common.HexToAddress("0x5ade") // contended recipient
	kA, _ := mkSender()
	kB, _ := mkSender()
	kC, _ := mkSender()
	uniq := common.HexToAddress("0xa11ce")

	// A and B both pay the shared recipient (mutually conflicting); A also has a
	// second, dependent transaction; C pays a unique recipient.
	txs := []*types.Transaction{
		tx(kA, 0, shared, 30),
		tx(kA, 1, uniq, 25),
		tx(kB, 0, shared, 20),
		tx(kC, 0, common.HexToAddress("0xb0b"), 15),
	}

	seq := buildComparableBlock(t, cfg, txs, alloc, false)
	par := buildComparableBlock(t, cfg, txs, alloc, true)

	if len(par.Transactions) != len(seq.Transactions) {
		t.Fatalf("tx count mismatch: parallel %d, sequential %d", len(par.Transactions), len(seq.Transactions))
	}
	if len(seq.Transactions) != len(txs) {
		t.Fatalf("expected all %d txs included, got %d", len(txs), len(seq.Transactions))
	}
	if par.GasUsed != seq.GasUsed {
		t.Fatalf("gas used mismatch: parallel %d, sequential %d", par.GasUsed, seq.GasUsed)
	}
	if par.StateRoot != seq.StateRoot {
		t.Fatalf("state root mismatch:\n parallel  %s\n sequential %s", par.StateRoot.Hex(), seq.StateRoot.Hex())
	}
}
