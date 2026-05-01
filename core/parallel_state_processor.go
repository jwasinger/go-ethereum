package core

import (
	"cmp"
	"fmt"
	"runtime"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/core/vm"
	"golang.org/x/sync/errgroup"
)

// ProcessResultWithMetrics wraps ProcessResult with some metrics that are
// emitted when executing blocks containing access lists.
type ProcessResultWithMetrics struct {
	ProcessResult          *ProcessResult
	PreProcessTime         time.Duration
	StateTransitionMetrics *state.BALStateTransitionMetrics
	// the time it took to execute all txs in the block
	ExecTime        time.Duration
	PostProcessTime time.Duration
	// Counts is the per-StateDB sum of state-mutation counters across pre-tx,
	// per-tx and post-tx phases. The caller may override AccountLoaded and
	// StorageLoaded with deduplicated counts derived from block.AccessList()
	// (per-StateDB sums over-count addresses touched by multiple phases).
	Counts state.StateCounts
	// Reads is the sum of per-StateDB read times across pre-tx, per-tx and
	// post-tx phases. Sum-of-CPU-time, not wall-clock.
	Reads state.ReadDurations
	// CodeLoaded is the deduplicated count of unique contract addresses whose
	// code body was fetched during the block (across all phase StateDBs).
	// CodeLoadBytes is the sum of those code lengths.
	CodeLoaded    int
	CodeLoadBytes int
}

// ParallelStateProcessor is used to execute and verify blocks containing
// access lists.
type ParallelStateProcessor struct {
	*StateProcessor
	vmCfg *vm.Config
}

// NewParallelStateProcessor returns a new ParallelStateProcessor instance.
func NewParallelStateProcessor(chain *HeaderChain, vmConfig *vm.Config) ParallelStateProcessor {
	res := NewStateProcessor(chain)
	return ParallelStateProcessor{
		res,
		vmConfig,
	}
}

func validateStateAccesses(lastIdx int, accessList bal.AccessListReader, localAccesses bal.StateAccesses) bool {
	// 1. strip out any state in the localAccesses that was modified
	muts := accessList.Mutations(lastIdx + 1)
	for acct, mut := range *muts {
		if _, exist := localAccesses[acct]; !exist {
			continue
		}
		// delete any storage slots that were mutated from the read set
		if len(localAccesses[acct]) > 0 {
			for key, _ := range mut.StorageWrites {
				if _, ok := localAccesses[acct][key]; ok {
					delete(localAccesses[acct], key)
				}
			}
		}

		if len(localAccesses[acct]) == 0 {
			delete(localAccesses, acct)
		}
	}
	if !accessList.Accesses().Eq(localAccesses) {
		return false
	}
	return true
}

// called by resultHandler when all transactions have successfully executed.
// performs post-tx state transition (system contracts and withdrawals)
// and calculates the ProcessResult, returning it to be sent on resCh
// by resultHandler
func (p *ParallelStateProcessor) prepareExecResult(block *types.Block, tExecStart time.Time, accesses bal.StateAccesses, statedb *state.StateDB, prefetchReader state.Reader, results []txExecResult, aggCounts state.StateCounts, aggReads state.ReadDurations, aggCodeLoads map[common.Address]int) *ProcessResultWithMetrics {
	tExec := time.Since(tExecStart)
	var requests [][]byte
	tPostprocessStart := time.Now()
	header := block.Header()

	context := NewEVMBlockContext(header, p.chain, nil)
	lastBALIdx := len(block.Transactions()) + 1
	postTxState := statedb.WithReader(state.NewReaderWithTracker(state.NewReaderWithBlockLevelAccessList(prefetchReader, *block.AccessList(), lastBALIdx)))

	cfg := vm.Config{
		NoBaseFee:               p.vmCfg.NoBaseFee,
		EnablePreimageRecording: p.vmCfg.EnablePreimageRecording,
		ExtraEips:               slices.Clone(p.vmCfg.ExtraEips),
	}
	evm := vm.NewEVM(context, postTxState, p.chainConfig(), cfg)

	// 1. order the receipts by tx index
	// 2. correctly calculate the cumulative gas used per receipt, returning bad block error if it goes over the allowed
	slices.SortFunc(results, func(a, b txExecResult) int {
		return cmp.Compare(a.receipt.TransactionIndex, b.receipt.TransactionIndex)
	})

	var (
		// Per-dimension cumulative sums for 2D block gas (EIP-8037).
		sumRegular        uint64
		sumState          uint64
		cumulativeReceipt uint64 // cumulative receipt gas (what users pay)
	)

	var allLogs []*types.Log
	var allReceipts []*types.Receipt
	for _, result := range results {
		sumRegular += result.txRegular
		sumState += result.txState

		cumulativeReceipt += result.execGas
		result.receipt.CumulativeGasUsed = cumulativeReceipt
		allLogs = append(allLogs, result.receipt.Logs...)
		allReceipts = append(allReceipts, result.receipt)
	}
	// Block gas = max(sum_regular, sum_state) per EIP-8037.
	blockGasUsed := max(sumRegular, sumState)
	if blockGasUsed > header.GasLimit {
		return &ProcessResultWithMetrics{
			ProcessResult: &ProcessResult{Error: fmt.Errorf("gas limit exceeded")},
		}
	}

	var postMut bal.StateMutations
	// Read requests if Prague is enabled.
	if p.chainConfig().IsPrague(block.Number(), block.Time()) {
		requests = [][]byte{}
		var err error
		// EIP-6110
		if err = ParseDepositLogs(&requests, allLogs, p.chainConfig()); err != nil {
			return &ProcessResultWithMetrics{
				ProcessResult: &ProcessResult{Error: err},
			}
		}

		// EIP-7002
		postMut, err = ProcessWithdrawalQueue(&requests, evm)
		if err != nil {
			return &ProcessResultWithMetrics{
				ProcessResult: &ProcessResult{Error: err},
			}
		}

		// EIP-7251
		consolidationMut, err := ProcessConsolidationQueue(&requests, evm)
		if err != nil {
			return &ProcessResultWithMetrics{
				ProcessResult: &ProcessResult{Error: err},
			}
		}
		postMut.Merge(consolidationMut)
	}

	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	postMut.Merge(p.chain.Engine().Finalize(p.chain, header, postTxState, block.Body()))
	postTxAccesses := postTxState.Reader().(state.StateReaderTracker).GetStateAccessList()

	accessList := bal.NewAccessListReader(*block.AccessList())
	if !postMut.Eq(*accessList.MutationsAt(lastBALIdx)) {
		return &ProcessResultWithMetrics{
			ProcessResult: &ProcessResult{Error: fmt.Errorf("invalid block access list: mismatch between local/remote access list mutations for final idx")},
		}
	}

	accesses.Merge(postTxAccesses)
	if !validateStateAccesses(lastBALIdx, accessList, accesses) {
		return &ProcessResultWithMetrics{
			ProcessResult: &ProcessResult{Error: fmt.Errorf("invalid block access list: mismatch between local/remote access list for state accesses")},
		}
	}

	tPostprocess := time.Since(tPostprocessStart)

	// Fold post-tx statedb counts and reads into the aggregate. postTxState is
	// local and would otherwise be discarded; this captures system-contract
	// activity (withdrawal queue, consolidation queue) and engine.Finalize.
	aggCounts.Add(postTxState.SnapshotCounts())
	aggReads.Add(postTxState.SnapshotReads())
	for addr, l := range postTxState.SnapshotCodeLoads() {
		if _, ok := aggCodeLoads[addr]; !ok {
			aggCodeLoads[addr] = l
		}
	}

	codeLoaded := len(aggCodeLoads)
	var codeLoadBytes int
	for _, l := range aggCodeLoads {
		codeLoadBytes += l
	}

	return &ProcessResultWithMetrics{
		ProcessResult: &ProcessResult{
			Receipts: allReceipts,
			Requests: requests,
			Logs:     allLogs,
			GasUsed:  blockGasUsed,
		},
		PostProcessTime: tPostprocess,
		ExecTime:        tExec,
		Counts:          aggCounts,
		Reads:           aggReads,
		CodeLoaded:      codeLoaded,
		CodeLoadBytes:   codeLoadBytes,
	}
}

type txExecResult struct {
	idx      int // transaction index
	receipt  *types.Receipt
	err      error // non-EVM error which would render the block invalid
	blockGas uint64
	execGas  uint64

	// Per-tx dimensional gas for Amsterdam 2D gas accounting (EIP-8037).
	txRegular uint64
	txState   uint64

	stateReads bal.StateAccesses

	// Per-tx state-mutation counts and read durations, snapshotted from the
	// worker statedb just before send. Aggregated single-threaded in
	// resultHandler.
	counts state.StateCounts
	reads  state.ReadDurations
	// codeLoads is addr→codeLen for contracts whose code body was fetched
	// in this tx. Deduped across all phases in resultHandler.
	codeLoads map[common.Address]int
}

// resultHandler polls until all transactions have finished executing and the
// state root calculation is complete. The result is emitted on resCh.
func (p *ParallelStateProcessor) resultHandler(block *types.Block, preTxAccesses bal.StateAccesses, preCounts state.StateCounts, preReads state.ReadDurations, preCodeLoads map[common.Address]int, statedb *state.StateDB, prefetchReader state.Reader, tExecStart time.Time, txResCh <-chan txExecResult, stateRootCalcResCh <-chan stateRootCalculationResult, resCh chan *ProcessResultWithMetrics) {
	// 1. if the block has transactions, receive the execution results from all of them and return an error on resCh if any txs err'd
	// 2. once all txs are executed, compute the post-tx state transition and produce the ProcessResult sending it on resCh (or an error if the post-tx state didn't match what is reported in the BAL)
	var results []txExecResult
	var cumulativeStateGas, cumulativeRegularGas uint64
	var execErr error
	var numTxComplete int

	// Seed aggregates with the pre-tx contribution (BeaconRoot, ParentBlockHash).
	// Per-tx fold below; post-tx fold in prepareExecResult.
	accesses := preTxAccesses
	aggCounts := preCounts
	aggReads := preReads
	// Dedup'd map of contract addresses whose code body was fetched by any
	// phase StateDB. Address-keyed so multiple phases adding the same contract
	// only count it once.
	aggCodeLoads := make(map[common.Address]int)
	for addr, l := range preCodeLoads {
		aggCodeLoads[addr] = l
	}

	if len(block.Transactions()) > 0 {
	loop:
		for {
			select {
			case res := <-txResCh:
				numTxComplete++
				if execErr == nil {
					// short-circuit if invalid block was detected
					if res.err != nil {
						execErr = res.err
					} else if bottleneck := max(cumulativeRegularGas+res.txRegular, cumulativeStateGas+res.txState); bottleneck > block.GasLimit() {
						execErr = fmt.Errorf("block used too much gas in bottleneck dimension: %d. block gas limit is %d", bottleneck, block.GasLimit())
					} else {
						cumulativeStateGas += res.txState
						results = append(results, res)
						accesses.Merge(res.stateReads)
						aggCounts.Add(res.counts)
						aggReads.Add(res.reads)
						for addr, l := range res.codeLoads {
							if _, ok := aggCodeLoads[addr]; !ok {
								aggCodeLoads[addr] = l
							}
						}
					}
				}
				if numTxComplete == len(block.Transactions()) {
					break loop
				}
			}
		}

		if execErr != nil {
			// Drain stateRootCalcResCh so calcAndVerifyRoot goroutine can exit.
			<-stateRootCalcResCh
			resCh <- &ProcessResultWithMetrics{ProcessResult: &ProcessResult{Error: execErr}}
			return
		}
	}

	execResults := p.prepareExecResult(block, tExecStart, accesses, statedb, prefetchReader, results, aggCounts, aggReads, aggCodeLoads)
	rootCalcRes := <-stateRootCalcResCh

	if execResults.ProcessResult.Error != nil {
		resCh <- execResults
	} else if rootCalcRes.err != nil {
		resCh <- &ProcessResultWithMetrics{ProcessResult: &ProcessResult{Error: rootCalcRes.err}}
	} else {
		execResults.StateTransitionMetrics = rootCalcRes.metrics
		resCh <- execResults
	}
}

type stateRootCalculationResult struct {
	err     error
	metrics *state.BALStateTransitionMetrics
}

// calcAndVerifyRoot performs the post-state root hash calculation, verifying
// it against what is reported by the block and returning a result on resCh.
func (p *ParallelStateProcessor) calcAndVerifyRoot(block *types.Block, stateTransition *state.BALStateTransition, resCh chan stateRootCalculationResult) {
	root := stateTransition.IntermediateRoot(false)

	res := stateRootCalculationResult{
		metrics: stateTransition.Metrics(),
	}

	if root != block.Root() {
		res.err = fmt.Errorf("state root mismatch. local: %x. remote: %x", root, block.Root())
	}
	resCh <- res
}

// execTx executes single transaction returning a result which includes state accessed/modified
func (p *ParallelStateProcessor) execTx(block *types.Block, tx *types.Transaction, balIdx int, db *state.StateDB, signer types.Signer) *txExecResult {
	header := block.Header()
	context := NewEVMBlockContext(header, p.chain, nil)

	cfg := vm.Config{
		NoBaseFee:               p.vmCfg.NoBaseFee,
		EnablePreimageRecording: p.vmCfg.EnablePreimageRecording,
		ExtraEips:               slices.Clone(p.vmCfg.ExtraEips),
	}
	evm := vm.NewEVM(context, db, p.chainConfig(), cfg)

	msg, err := TransactionToMessage(tx, signer, header.BaseFee)
	if err != nil {
		err = fmt.Errorf("could not apply tx %d [%v]: %w", balIdx, tx.Hash().Hex(), err)
		return &txExecResult{err: err}
	}
	gp := NewGasPool(block.GasLimit())
	db.SetTxContext(tx.Hash(), balIdx-1)

	mut, receipt, err := ApplyTransactionWithEVM(msg, gp, db, block.Number(), block.Hash(), context.Time, tx, evm)
	if err != nil {
		err := fmt.Errorf("could not apply tx %d [%v]: %w", balIdx, tx.Hash().Hex(), err)
		return &txExecResult{err: err}
	}

	accessList := bal.NewAccessListReader(*block.AccessList())
	if !accessList.MutationsAt(balIdx).Eq(mut) {
		err := fmt.Errorf("invalid block access list: mismatch between local/remote computed state mutations at bal idx %d. got:\n%s\nexpected:\n%s\n", balIdx, mut.String(), accessList.MutationsAt(balIdx).String())
		return &txExecResult{err: err}
	}

	txRegular, txState := gp.AmsterdamDimensions()
	return &txExecResult{
		idx:        balIdx,
		receipt:    receipt,
		execGas:    receipt.GasUsed,
		blockGas:   gp.Used(),
		txRegular:  txRegular,
		txState:    txState,
		stateReads: db.Reader().(state.StateReaderTracker).GetStateAccessList(),
		counts:     db.SnapshotCounts(),
		reads:      db.SnapshotReads(),
		codeLoads:  db.SnapshotCodeLoads(),
	}
}

func (p *ParallelStateProcessor) processBlockPreTx(block *types.Block, statedb *state.StateDB, prefetchReader state.Reader, cfg vm.Config) (bal.StateAccesses, state.StateCounts, state.ReadDurations, map[common.Address]int, error) {
	var (
		header = block.Header()
	)

	alReader := state.NewReaderWithBlockLevelAccessList(prefetchReader, *block.AccessList(), 0)
	readerWithTracker := state.NewReaderWithTracker(alReader)
	sdb := statedb.WithReader(readerWithTracker)
	accessList := bal.NewAccessListReader(*block.AccessList())

	context := NewEVMBlockContext(header, p.chain, nil)
	evm := vm.NewEVM(context, sdb, p.chainConfig(), cfg)

	var mutations bal.StateMutations
	if beaconRoot := block.BeaconRoot(); beaconRoot != nil {
		mutations = ProcessBeaconBlockRoot(*beaconRoot, evm)
	}

	pbhMutations := ProcessParentBlockHash(block.ParentHash(), evm)
	mutations.Merge(pbhMutations)
	reads := readerWithTracker.(state.StateReaderTracker).GetStateAccessList()
	if !accessList.MutationsAt(0).Eq(mutations) {
		return nil, state.StateCounts{}, state.ReadDurations{}, nil, fmt.Errorf("invalid block access list: mismatch between local/remote access list mutations at idx 0")
	}
	// Snapshot the pre-tx statedb's counts/reads/code-loads so system-contract
	// activity (BeaconRoot, ParentBlockHash) contributes to the aggregate;
	// sdb is local and would otherwise be discarded.
	return reads, sdb.SnapshotCounts(), sdb.SnapshotReads(), sdb.SnapshotCodeLoads(), nil
}

// Process performs EVM execution and state root computation for a block which is known
// to contain an access list.
func (p *ParallelStateProcessor) Process(block *types.Block, stateTransition *state.BALStateTransition, statedb *state.StateDB, cfg vm.Config) (*ProcessResultWithMetrics, error) {
	var (
		header           = block.Header()
		resCh            = make(chan *ProcessResultWithMetrics)
		signer           = types.MakeSigner(p.chainConfig(), header.Number, header.Time)
		rootCalcResultCh = make(chan stateRootCalculationResult)
		txResCh          = make(chan txExecResult)

		pStart      = time.Now()
		tExecStart  time.Time
		tPreprocess time.Duration // time to create a set of prestates for parallel transaction execution
		balReader   = statedb.Reader()
	)

	startingState := statedb.Copy()
	preTxReads, preCounts, preReads, preCodeLoads, err := p.processBlockPreTx(block, statedb, balReader, cfg)
	if err != nil {
		return nil, err
	}

	// compute the reads/mutations at the last bal index
	tPreprocess = time.Since(pStart)

	// execute transactions and state root calculation in parallel
	tExecStart = time.Now()
	go p.resultHandler(block, preTxReads, preCounts, preReads, preCodeLoads, statedb, balReader, tExecStart, txResCh, rootCalcResultCh, resCh)
	var workers errgroup.Group
	workers.SetLimit(runtime.NumCPU())
	for i, t := range block.Transactions() {
		tx := t
		idx := i
		sdb := startingState.Copy()
		workers.Go(func() error {
			startingStateWithReadTracker := sdb.WithReader(state.NewReaderWithTracker(state.NewReaderWithBlockLevelAccessList(balReader, *block.AccessList(), idx+1)))
			res := p.execTx(block, tx, idx+1, startingStateWithReadTracker, signer)
			txResCh <- *res
			return nil
		})
	}

	go p.calcAndVerifyRoot(block, stateTransition, rootCalcResultCh)

	res := <-resCh
	if res.ProcessResult.Error != nil {
		return nil, res.ProcessResult.Error
	}
	// TODO: remove preprocess metric ?
	res.PreProcessTime = tPreprocess
	return res, nil
}
