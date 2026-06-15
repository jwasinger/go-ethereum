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
	"crypto/sha256"
	"math/big"
	"runtime"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// benchBlockGasLimit is the target gas limit for the block being built.
	benchBlockGasLimit = 100_000_000
	// benchPointEvals is the number of point-evaluation precompile calls each
	// transaction performs. At 50,000 gas per call this dominates each
	// transaction's gas cost and makes execution genuinely compute-bound.
	benchPointEvals = 128
	// benchTxGasLimit must cover benchPointEvals calls plus per-call overhead.
	benchTxGasLimit = 7_000_000
	// benchNumTxs is chosen so the cumulative gas comfortably exceeds the block
	// gas limit, ensuring the block is packed full.
	benchNumTxs = 16
)

// pointEvalContractAddr is where the KZG-precompile-spamming contract lives.
var pointEvalContractAddr = common.HexToAddress("0xc0de")

// validPointEvalInput builds a 192-byte input the point-evaluation precompile
// (0x0a) accepts, so each call performs the full (expensive) KZG verification:
//
//	versioned_hash (32) | z (32) | y (32) | commitment (48) | proof (48)
func validPointEvalInput(b *testing.B) []byte {
	b.Helper()
	var blob kzg4844.Blob // the all-zero blob is valid
	commitment, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		b.Fatalf("BlobToCommitment: %v", err)
	}
	var z kzg4844.Point // evaluate at 0 (a valid field element)
	proof, claim, err := kzg4844.ComputeProof(&blob, z)
	if err != nil {
		b.Fatalf("ComputeProof: %v", err)
	}
	vh := kzg4844.CalcBlobHashV1(sha256.New(), &commitment)

	// Ensure the proof actually verifies, so the precompile reaches (and runs)
	// the expensive pairing check rather than erroring out early.
	if err := kzg4844.VerifyProof(commitment, z, claim, proof); err != nil {
		b.Fatalf("VerifyProof: %v", err)
	}

	input := make([]byte, 192)
	copy(input[0:32], vh[:])
	copy(input[32:64], z[:])
	copy(input[64:96], claim[:])
	copy(input[96:144], commitment[:])
	copy(input[144:192], proof[:])
	return input
}

// pointEvalLoopCode returns runtime bytecode that copies the embedded 192-byte
// precompile input into memory once, then STATICCALLs the point-evaluation
// precompile (0x0a) n times in a loop. The input blob is appended to the code
// and loaded via CODECOPY.
func pointEvalLoopCode(n uint16, input []byte) []byte {
	// Fixed-layout template; the three operands marked below are patched after
	// the offsets are known. See the byte offsets in the comments.
	code := []byte{
		0x60, 0xC0, // [0]  PUSH1 192            (CODECOPY size)
		0x61, 0x00, 0x00, // [2]  PUSH2 inputOff (CODECOPY src, patched at 3..4)
		0x60, 0x00, // [5]  PUSH1 0              (CODECOPY dest)
		0x39,             // [7]  CODECOPY
		0x61, 0x00, 0x00, // [8]  PUSH2 n        (loop counter, patched at 9..10)
		0x5B,       // [11] JUMPDEST  <- loopStart
		0x60, 0x40, // [12] PUSH1 64             (ret size)
		0x60, 0xC0, // [14] PUSH1 192            (ret offset)
		0x60, 0xC0, // [16] PUSH1 192            (args size)
		0x60, 0x00, // [18] PUSH1 0              (args offset)
		0x60, 0x0A, // [20] PUSH1 0x0a           (precompile address)
		0x5A,       // [22] GAS
		0xFA,       // [23] STATICCALL
		0x50,       // [24] POP             (discard success flag)
		0x60, 0x01, // [25] PUSH1 1
		0x90,             // [27] SWAP1
		0x03,             // [28] SUB             (counter = counter - 1)
		0x80,             // [29] DUP1
		0x61, 0x00, 0x0B, // [30] PUSH2 loopStart (patched at 31..32)
		0x57, // [33] JUMPI
		0x00, // [34] STOP
	}
	inputOff := uint16(len(code)) // input blob is appended directly after STOP
	code[3], code[4] = byte(inputOff>>8), byte(inputOff)
	code[9], code[10] = byte(n>>8), byte(n)
	// loopStart is 11; already encoded as 0x000B at [31..32].
	return append(code, input...)
}

// setupHeavyBlock builds an Amsterdam genesis with the KZG-spamming contract
// pre-deployed and benchNumTxs distinct senders, returning the chain config,
// genesis spec, and the signed transactions (each calling the contract).
func setupHeavyBlock(b *testing.B) (*params.ChainConfig, *core.Genesis, []*types.Transaction) {
	b.Helper()
	cfg := amsterdamConfig()
	signer := types.LatestSigner(cfg)
	input := validPointEvalInput(b)
	code := pointEvalLoopCode(benchPointEvals, input)

	alloc := types.GenesisAlloc{
		params.BeaconRootsAddress:        {Nonce: 1, Code: params.BeaconRootsCode, Balance: common.Big0},
		params.HistoryStorageAddress:     {Nonce: 1, Code: params.HistoryStorageCode, Balance: common.Big0},
		params.WithdrawalQueueAddress:    {Nonce: 1, Code: params.WithdrawalQueueCode, Balance: common.Big0},
		params.ConsolidationQueueAddress: {Nonce: 1, Code: params.ConsolidationQueueCode, Balance: common.Big0},
		pointEvalContractAddr:            {Nonce: 1, Code: code, Balance: common.Big0},
	}

	tip := big.NewInt(params.GWei)
	feeCap := new(big.Int).Add(big.NewInt(params.InitialBaseFee), tip)
	var txs []*types.Transaction
	for i := 0; i < benchNumTxs; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)
		alloc[addr] = types.Account{Balance: new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))}
		tx := types.MustSignNewTx(key, signer, &types.DynamicFeeTx{
			ChainID:   cfg.ChainID,
			Nonce:     0,
			To:        &pointEvalContractAddr,
			Gas:       benchTxGasLimit,
			GasTipCap: tip,
			GasFeeCap: feeCap,
		})
		txs = append(txs, tx)
	}
	gspec := &core.Genesis{
		Config:   cfg,
		Alloc:    alloc,
		GasLimit: benchBlockGasLimit,
		BaseFee:  big.NewInt(params.InitialBaseFee),
	}
	return cfg, gspec, txs
}

// benchmarkBuildBlock builds a full ~100Mgas block of compute-heavy
// transactions b.N times, using either the conflict-aware parallel builder or
// the sequential ("legacy") builder.
func benchmarkBuildBlock(b *testing.B, parallel bool) {
	_, gspec, txs := setupHeavyBlock(b)
	runBuildBenchmark(b, gspec, txs, testTxPoolConfig, parallel)
}

// runBuildBenchmark wires up a fresh chain, txpool, and miner from the given
// genesis and transactions, then repeatedly builds a block using either the
// parallel or the sequential builder. The pool config is supplied by the caller
// because different workloads need different pool capacities (a few large
// transactions vs. thousands of small ones).
func runBuildBenchmark(b *testing.B, gspec *core.Genesis, txs []*types.Transaction, poolCfg legacypool.Config, parallel bool) {
	engine := beacon.New(ethash.NewFaker())
	db := rawdb.NewMemoryDatabase()
	chain, err := core.NewBlockChain(db, gspec, engine, &core.BlockChainConfig{ArchiveMode: true})
	if err != nil {
		b.Fatalf("NewBlockChain: %v", err)
	}
	defer chain.Stop()

	pool := legacypool.New(poolCfg, chain)
	pl, err := txpool.New(poolCfg.PriceLimit, chain, []txpool.SubPool{pool})
	if err != nil {
		b.Fatalf("txpool.New: %v", err)
	}
	defer pl.Close()

	cfg := testConfig
	cfg.ParallelBuild = parallel
	cfg.GasCeil = benchBlockGasLimit
	// Build the entire block without the recommit timeout interrupting us, so
	// both builders produce a full block and the comparison measures the same
	// amount of work.
	cfg.Recommit = time.Hour
	backend := &testWorkerBackend{db: db, chain: chain, txPool: pl, genesis: gspec}
	miner := New(backend, cfg, engine)

	if errs := pl.Add(txs, true); len(errs) > 0 {
		for _, e := range errs {
			if e != nil {
				b.Fatalf("txpool add: %v", e)
			}
		}
	}
	// Wait until enough transactions are pending to fill the block.
	want := len(txs)
	for i := 0; i < 400; i++ {
		n := 0
		p, _ := pl.Pending(txpool.PendingFilter{})
		for _, ts := range p {
			n += len(ts)
		}
		if n >= want {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	slot := uint64(1)
	parent := chain.CurrentBlock()
	genParams := &generateParams{
		timestamp:  parent.Time + 12,
		forceTime:  true,
		parentHash: parent.Hash(),
		coinbase:   common.HexToAddress("0xc01b"),
		random:     common.Hash{0x1},
		slotNum:    &slot,
		noTxs:      false,
	}

	// Sanity build to confirm the block is actually packed full before timing.
	res := miner.generateWork(context.Background(), genParams, false)
	if res.err != nil {
		b.Fatalf("generateWork: %v", res.err)
	}
	b.ReportMetric(float64(res.block.GasUsed()), "blockGasUsed")
	b.ReportMetric(float64(len(res.block.Transactions())), "txs")
	b.Logf("packed %d txs, gasUsed=%d / limit=%d (%.1f%%), workers=%d",
		len(res.block.Transactions()), res.block.GasUsed(), res.block.GasLimit(),
		100*float64(res.block.GasUsed())/float64(res.block.GasLimit()), runtime.NumCPU())

	b.ResetTimer()
	var totalGas uint64
	for i := 0; i < b.N; i++ {
		r := miner.generateWork(context.Background(), genParams, false)
		if r.err != nil {
			b.Fatalf("generateWork: %v", r.err)
		}
		totalGas += r.block.GasUsed()
	}
	b.StopTimer()

	// Report build throughput in millions of gas per second, aggregated over
	// all benchmark iterations. This is the headline number for comparing the
	// builders: how much block gas each can pack per unit of wall-clock time.
	if secs := b.Elapsed().Seconds(); secs > 0 {
		b.ReportMetric(float64(totalGas)/secs/1e6, "Mgas/s")
	}
}

// BenchmarkBuildBlockParallel builds the block with the conflict-aware parallel
// construction path enabled.
func BenchmarkBuildBlockParallel(b *testing.B) { benchmarkBuildBlock(b, true) }

// BenchmarkBuildBlockLegacy builds the same block with the sequential
// construction path.
func BenchmarkBuildBlockLegacy(b *testing.B) { benchmarkBuildBlock(b, false) }

// benchEOATxCount is the number of EOA transfers prepared for the EOA-fill
// benchmark. At 21,000 gas each, ~4,762 fill a 100M-gas block; a margin is
// added so the block packs completely full.
const benchEOATxCount = 5200

// setupEOABlock builds an Amsterdam genesis with benchEOATxCount distinct
// senders, each making a single simple value transfer to a distinct recipient.
// Distinct senders and recipients make every transaction non-conflicting.
func setupEOABlock(b *testing.B) (*core.Genesis, []*types.Transaction) {
	b.Helper()
	cfg := amsterdamConfig()
	signer := types.LatestSigner(cfg)

	alloc := types.GenesisAlloc{
		params.BeaconRootsAddress:        {Nonce: 1, Code: params.BeaconRootsCode, Balance: common.Big0},
		params.HistoryStorageAddress:     {Nonce: 1, Code: params.HistoryStorageCode, Balance: common.Big0},
		params.WithdrawalQueueAddress:    {Nonce: 1, Code: params.WithdrawalQueueCode, Balance: common.Big0},
		params.ConsolidationQueueAddress: {Nonce: 1, Code: params.ConsolidationQueueCode, Balance: common.Big0},
	}
	tip := big.NewInt(params.GWei)
	feeCap := new(big.Int).Add(big.NewInt(params.InitialBaseFee), tip)
	funding := new(big.Int).Mul(big.NewInt(params.GWei), big.NewInt(params.GWei)) // 1e18 wei = 1 ETH

	txs := make([]*types.Transaction, 0, benchEOATxCount)
	for i := 0; i < benchEOATxCount; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)
		alloc[addr] = types.Account{Balance: funding}
		// Distinct recipient per transaction, offset well clear of precompile
		// and system-contract addresses.
		recipient := common.BigToAddress(new(big.Int).SetUint64(uint64(i) + (1 << 20)))
		tx := types.MustSignNewTx(key, signer, &types.DynamicFeeTx{
			ChainID:   cfg.ChainID,
			Nonce:     0,
			To:        &recipient,
			Value:     big.NewInt(1),
			Gas:       params.TxGas,
			GasTipCap: tip,
			GasFeeCap: feeCap,
		})
		txs = append(txs, tx)
	}
	gspec := &core.Genesis{
		Config:   cfg,
		Alloc:    alloc,
		GasLimit: benchBlockGasLimit,
		BaseFee:  big.NewInt(params.InitialBaseFee),
	}
	return gspec, txs
}

// eosPoolConfig returns a legacypool config sized to hold all of the EOA-fill
// benchmark's transactions as pending simultaneously (the default limits are
// far smaller than the thousands of transactions needed to fill the block).
func eoaPoolConfig() legacypool.Config {
	cfg := testTxPoolConfig
	cfg.GlobalSlots = benchEOATxCount + 1024
	cfg.GlobalQueue = benchEOATxCount + 1024
	cfg.AccountSlots = 16
	cfg.AccountQueue = 64
	return cfg
}

// benchmarkBuildEOABlock fills a full 100M-gas block with ~4,760 simple value
// transfers. This is the opposite extreme from the KZG benchmark: the
// transactions are individually trivial, so per-transaction overhead dominates.
//
// Note the parallel builder is expected to be far SLOWER than the legacy
// builder here: it speculatively executes each candidate against its own deep
// copy of the pending state, and filling the block takes dozens of rounds of 64
// candidates over a state that grows with every committed transaction. For
// cheap transactions that copy cost dwarfs the (tiny) execution it parallelises.
// The legacy builder fills the same block in a few hundred milliseconds.
func benchmarkBuildEOABlock(b *testing.B, parallel bool) {
	gspec, txs := setupEOABlock(b)
	runBuildBenchmark(b, gspec, txs, eoaPoolConfig(), parallel)
}

// BenchmarkBuildEOABlockParallel fills a 100M-gas block with simple EOA
// transfers using the parallel builder.
func BenchmarkBuildEOABlockParallel(b *testing.B) { benchmarkBuildEOABlock(b, true) }

// BenchmarkBuildEOABlockLegacy fills the same block of EOA transfers using the
// sequential builder.
func BenchmarkBuildEOABlockLegacy(b *testing.B) { benchmarkBuildEOABlock(b, false) }
