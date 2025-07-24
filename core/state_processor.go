// Copyright 2015 The go-ethereum Authors
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
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	chain  *HeaderChain        // Canonical header chain
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, chain *HeaderChain) *StateProcessor {
	return &StateProcessor{
		config: config,
		chain:  chain,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (*ProcessResult, error) {
	var (
		receipts    types.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		gp          = new(GasPool).AddGas(block.GasLimit())
	)

	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	var (
		context vm.BlockContext
		signer  = types.MakeSigner(p.config, header.Number, header.Time)
	)

	// Apply pre-execution system calls.
	var tracingStateDB = vm.StateDB(statedb)
	if hooks := cfg.Tracer; hooks != nil {
		tracingStateDB = state.NewHookedState(statedb, hooks)
	}
	context = NewEVMBlockContext(header, p.chain, nil)
	evm := vm.NewEVM(context, tracingStateDB, p.config, cfg)

	// process beacon-root and parent block system contracts.
	// do not include the storage writes in the BAL:
	// * beacon root will be provided as a standalone field in the BAL
	// * parent block hash is already in the header field of the block

	// TODO: use TxContext (hash == common.Hash{}) as a signal that we aren't
	// executing a tx yet, and don't record state based on that?
	if statedb.BlockAccessList() != nil {
		statedb.BlockAccessList().DisableMutations()
	}
	if beaconRoot := block.BeaconRoot(); beaconRoot != nil {
		ProcessBeaconBlockRoot(*beaconRoot, evm)
	}
	if p.config.IsPrague(block.Number(), block.Time()) || p.config.IsVerkle(block.Number(), block.Time()) {
		ProcessParentBlockHash(block.ParentHash(), evm)
	}
	if statedb.BlockAccessList() != nil {
		statedb.BlockAccessList().EnableMutations()
	}

	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		msg, err := TransactionToMessage(tx, signer, header.BaseFee)
		if err != nil {

			return nil, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}

		sender, _ := types.Sender(signer, tx)
		statedb.SetTxSender(sender)
		statedb.SetTxContext(tx.Hash(), i)

		_, receipt, err := ApplyTransactionWithEVM(msg, gp, statedb, blockNumber, blockHash, context.Time, tx, usedGas, evm, nil)
		if err != nil {
			return nil, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}

	// TODO: note that the below clause is only for BAL building.  Perhaps use the idea I showed above to remove explicit call to disable mutations
	// don't write post-block state mutations to the BAL to save on size.
	// these can be easily computed in BAL verification.
	if statedb.BlockAccessList() != nil {
		statedb.BlockAccessList().DisableMutations()
	}
	// Read requests if Prague is enabled.
	var requests [][]byte
	if p.config.IsPrague(block.Number(), block.Time()) {
		requests = [][]byte{}
		// EIP-6110
		if err := ParseDepositLogs(&requests, allLogs, p.config); err != nil {
			return nil, err
		}
		// EIP-7002
		if err := ProcessWithdrawalQueue(&requests, evm); err != nil {
			return nil, err
		}
		// EIP-7251
		if err := ProcessConsolidationQueue(&requests, evm); err != nil {
			return nil, err
		}
	}

	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.chain.engine.Finalize(p.chain, header, tracingStateDB, block.Body())

	return &ProcessResult{
		Receipts: receipts,
		Requests: requests,
		Logs:     allLogs,
		GasUsed:  *usedGas,
	}, nil
}

func (p *StateProcessor) calcStateDiffs(evm *vm.EVM, block *types.Block, txPrestate *state.StateDB) (totalDiff *bal.StateDiff, txDiffs []*bal.StateDiff, err error) {
	prestateDiff := txPrestate.GetStateDiff()
	// create a number of diffs (one for each worker goroutine)
	txDiffIt := bal.NewIterator(block.Body().AccessList, len(block.Transactions()))
	header := block.Header()
	signer := types.MakeSigner(p.config, header.Number, header.Time)
	return txDiffIt.BuildStateDiffs(prestateDiff, uint16(len(block.Transactions()))-1, func(txIndex uint16, accumDiff, txDiff *bal.StateDiff) error {
		// create the complete account tx post-state by filling in values that the BAL does not provide:
		// * tx sender post nonce: infer from the transaction for non-delegated EOAs
		// * 7702 delegation code changes: infer from the delegations in the transaction and the tx pre-state balances

		stateReader := state.NewBALStateReader(txPrestate, accumDiff)

		tx := block.Transactions()[txIndex]
		sender, err := types.Sender(signer, tx)
		if err != nil {
			panic("what could cause the error here?!?!")
		}

		// fill in the tx sender nonce
		if senderDiff, ok := txDiff.Mutations[sender]; ok {
			// if the sender account nonce diff is set, it is a delegated EOA
			// which performed one or more creations, so the post-tx nonce value
			// will be recorded in the BAL
			if senderDiff.Nonce == nil {
				// TODO: can infer the new nonce from the transaction
				senderPostNonce := stateReader.GetNonce(sender) + 1
				senderDiff.Nonce = &senderPostNonce
			}
		} else {
			// TODO: can infer it from the transaction
			senderPostNonce := stateReader.GetNonce(sender) + 1

			as := bal.NewEmptyAccountState()
			as.Nonce = &senderPostNonce
			txDiff.Mutations[sender] = as
		}

		// for each delegation in the tx: calc if it has enough funds for the delegation to proceed and adjust the delegation target in the diff if so
		for _, delegation := range tx.SetCodeAuthorizations() {
			// TODO: don't blindly-assume that the delegation will succeed.  Validate which authorizations can succeed/fail based on the computed tx prestate
			// TODO: don't set the code directly (delegations are not charged by code size so the state diff can get very large in a block with lots of auths)

			authority, err := delegation.Authority()
			if err != nil {
				panic(err)
				continue
			}
			authority, err = validateAuth(&delegation, evm, stateReader)
			if err != nil {
				continue // auth validation failures don't constitute a bad block
			}

			if accountDiff, ok := txDiff.Mutations[authority]; ok {
				if accountDiff.Code != nil {
					panic("bad block: BAL included a code change at the authority address for this tx")
				}
				if delegation.Address == (common.Address{}) {
					accountDiff.Code = make(bal.ContractCode, 0)
				} else {
					accountDiff.Code = types.AddressToDelegation(delegation.Address)
				}

				if accountDiff.Nonce == nil {
					newNonce := delegation.Nonce + 1
					accountDiff.Nonce = &newNonce
				} else {
					// TODO: Check something here?  that the nonce is greater than the minimum?
				}
			} else {
				as := bal.NewEmptyAccountState()
				if delegation.Address == (common.Address{}) {
					accountDiff.Code = make(bal.ContractCode, 0)
				} else {
					accountDiff.Code = types.AddressToDelegation(delegation.Address)
				}
				newNonce := delegation.Nonce + 1
				as.Nonce = &newNonce
				txDiff.Mutations[authority] = as
			}
		}

		return nil
	})
}

func (p *StateProcessor) ProcessWithAccessList(block *types.Block, statedb *state.StateDB, cfg vm.Config, al *bal.BlockAccessList) (*state.StateDB, *bal.StateDiff, *ProcessResult, error) {
	var (
		receipts    types.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		gp          = new(GasPool).AddGas(block.GasLimit())
	)

	prestate := statedb.Copy()

	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	var (
		context vm.BlockContext
		signer  = types.MakeSigner(p.config, header.Number, header.Time)
	)

	// Apply pre-execution system calls.
	var tracingStateDB = vm.StateDB(statedb)
	if hooks := cfg.Tracer; hooks != nil {
		tracingStateDB = state.NewHookedState(statedb, hooks)
	}
	context = NewEVMBlockContext(header, p.chain, nil)
	evm := vm.NewEVM(context, tracingStateDB, p.config, cfg)

	// process beacon-root and parent block system contracts.
	// do not include the storage writes in the BAL:
	// * beacon root will be provided as a standalone field in the BAL
	// * parent block hash is already in the header field of the block

	// TODO: use TxContext (hash == common.Hash{}) as a signal that we aren't
	// executing a tx yet, and don't record state based on that?
	if statedb.BlockAccessList() != nil {
		statedb.BlockAccessList().DisableMutations()
	}

	if beaconRoot := block.BeaconRoot(); beaconRoot != nil {
		ProcessBeaconBlockRoot(*beaconRoot, evm)
	}
	// TODO: set the beacon root on the BAL if we are building a BAL
	if p.config.IsPrague(block.Number(), block.Time()) || p.config.IsVerkle(block.Number(), block.Time()) {
		ProcessParentBlockHash(block.ParentHash(), evm)
	}
	if statedb.BlockAccessList() != nil {
		statedb.BlockAccessList().EnableMutations()
	}

	postTxDiff := &bal.StateDiff{make(map[common.Address]*bal.AccountState)}

	if len(block.Transactions()) > 0 {
		var (
			stateDiffs []*bal.StateDiff
			err        error
		)
		postTxDiff, stateDiffs, err = p.calcStateDiffs(evm, block, statedb)
		if err != nil {
			panic("bad block error here")
		}

		// TODO: validate that system address execution changes aren't recorded in the BAL
		// unless triggered by a non-system contract (sending a balance to a sys address for example)

		txExecBALIt := bal.NewIterator(al, len(block.Transactions()))
		var balStateTxDiff *bal.StateDiff
		// Iterate over and process the individual transactions
		for i, tx := range block.Transactions() {
			msg, err := TransactionToMessage(tx, signer, header.BaseFee)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
			}
			sender, _ := types.Sender(signer, tx)
			statedb.SetTxSender(sender)
			statedb.SetTxContext(tx.Hash(), i)

			senderPreNonce := statedb.GetNonce(sender)
			cpy := statedb.Copy()
			evm.StateDB = cpy
			txStateDiff, receipt, err := ApplyTransactionWithEVM(msg, gp, cpy, blockNumber, blockHash, context.Time, tx, usedGas, evm, nil)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
			}

			receipts = append(receipts, receipt)
			allLogs = append(allLogs, receipt.Logs...)

			statedb.ApplyDiff(stateDiffs[i])
			// TODO: make ApplyDiff invoke Finalise
			statedb.Finalise(true, nil)
			balStateTxDiff = txExecBALIt.Next()

			// TODO validate the reported state diff with the produced one:
			// every entry in the reported diff should be in the produced one
			// the only extra entries in the produced diff should be tx sender nonce increment (if non-delegated), and delegation code changes (if successful)

			if err := bal.ValidateTxStateDiff(balStateTxDiff, txStateDiff, sender, senderPreNonce); err != nil {
				return nil, nil, nil, err
			}
		}

		statedb.ApplyDiff(postTxDiff)
	}

	// TODO: note that the below clause is only for BAL building.  Perhaps use the idea I showed above to remove explicit call to disable mutations
	// don't write post-block state mutations to the BAL to save on size.
	// these can be easily computed in BAL verification.
	if statedb.BlockAccessList() != nil {
		statedb.BlockAccessList().DisableMutations()
	}
	// Read requests if Prague is enabled.
	var requests [][]byte
	if p.config.IsPrague(block.Number(), block.Time()) {
		requests = [][]byte{}
		// EIP-6110
		if err := ParseDepositLogs(&requests, allLogs, p.config); err != nil {
			return nil, nil, nil, err
		}
		// EIP-7002
		if err := ProcessWithdrawalQueue(&requests, evm); err != nil {
			return nil, nil, nil, err
		}
		// EIP-7251
		if err := ProcessConsolidationQueue(&requests, evm); err != nil {
			return nil, nil, nil, err
		}
	}

	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	// TODO: apply withdrawals state diff from the Finalize call
	p.chain.engine.Finalize(p.chain, header, tracingStateDB, block.Body())
	// invoke Finalise so that withdrawals are accounted for in the state diff
	statedb.Finalise(true, nil)

	postTxDiff.Merge(statedb.GetStateDiff())

	processResult := &ProcessResult{
		Receipts: receipts,
		Requests: requests,
		Logs:     allLogs,
		GasUsed:  *usedGas,
	}

	return prestate, postTxDiff, processResult, nil
}

// ApplyTransactionWithEVM attempts to apply a transaction to the given state database
// and uses the input parameters for its environment similar to ApplyTransaction. However,
// this method takes an already created EVM instance as input.
func ApplyTransactionWithEVM(msg *Message, gp *GasPool, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, blockTime uint64, tx *types.Transaction, usedGas *uint64, evm *vm.EVM, balDiff *bal.StateDiff) (diff *bal.StateDiff, receipt *types.Receipt, err error) {
	if hooks := evm.Config.Tracer; hooks != nil {
		if hooks.OnTxStart != nil {
			hooks.OnTxStart(evm.GetVMContext(), tx, msg.From)
		}
		if hooks.OnTxEnd != nil {
			defer func() { hooks.OnTxEnd(receipt, err) }()
		}
	}
	// Apply the transaction to the current state (included in the env).
	result, err := ApplyMessage(evm, msg, gp)
	if err != nil {
		return nil, nil, err
	}

	// Update the state with pending changes.
	var root []byte
	if evm.ChainConfig().IsByzantium(blockNumber) {
		// TODO: when executing BAL here, the returned diff includes the transaction prestate when it should only return the state accessed+modified by the transaction
		//panic("fixme")
		diff, err = evm.StateDB.Finalise(true, balDiff)
		if err != nil {
			return nil, nil, err
		}
	} else {
		root = statedb.IntermediateRoot(evm.ChainConfig().IsEIP158(blockNumber)).Bytes()
	}
	*usedGas += result.UsedGas

	// Merge the tx-local access event into the "block-local" one, in order to collect
	// all values, so that the witness can be built.
	if statedb.Database().TrieDB().IsVerkle() {
		statedb.AccessEvents().Merge(evm.AccessEvents)
	}
	return diff, MakeReceipt(evm, result, statedb, blockNumber, blockHash, blockTime, tx, *usedGas, root), nil
}

// MakeReceipt generates the receipt object for a transaction given its execution result.
func MakeReceipt(evm *vm.EVM, result *ExecutionResult, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, blockTime uint64, tx *types.Transaction, usedGas uint64, root []byte) *types.Receipt {
	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &types.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: usedGas}
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas

	if tx.Type() == types.BlobTxType {
		receipt.BlobGasUsed = uint64(len(tx.BlobHashes()) * params.BlobTxBlobGasPerBlob)
		receipt.BlobGasPrice = evm.Context.BlobBaseFee
	}

	// If the transaction created a contract, store the creation address in the receipt.
	if tx.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, tx.Nonce())
	}

	// Set the receipt logs and create the bloom filter.
	receipt.Logs = statedb.GetLogs(tx.Hash(), blockNumber.Uint64(), blockHash, blockTime)
	receipt.Bloom = types.CreateBloom(receipt)
	receipt.BlockHash = blockHash
	receipt.BlockNumber = blockNumber
	receipt.TransactionIndex = uint(statedb.TxIndex())
	return receipt
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(evm *vm.EVM, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64) (*types.Receipt, error) {
	msg, err := TransactionToMessage(tx, types.MakeSigner(evm.ChainConfig(), header.Number, header.Time), header.BaseFee)
	if err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	_, receipts, err := ApplyTransactionWithEVM(msg, gp, statedb, header.Number, header.Hash(), header.Time, tx, usedGas, evm, nil)
	return receipts, err
}

// ProcessBeaconBlockRoot applies the EIP-4788 system call to the beacon block root
// contract. This method is exported to be used in tests.
func ProcessBeaconBlockRoot(beaconRoot common.Hash, evm *vm.EVM) *bal.StateDiff {
	if tracer := evm.Config.Tracer; tracer != nil {
		onSystemCallStart(tracer, evm.GetVMContext())
		if tracer.OnSystemCallEnd != nil {
			defer tracer.OnSystemCallEnd()
		}
	}
	msg := &Message{
		From:      params.SystemAddress,
		GasLimit:  30_000_000,
		GasPrice:  common.Big0,
		GasFeeCap: common.Big0,
		GasTipCap: common.Big0,
		To:        &params.BeaconRootsAddress,
		Data:      beaconRoot[:],
	}
	evm.SetTxContext(NewEVMTxContext(msg))
	evm.StateDB.AddAddressToAccessList(params.BeaconRootsAddress)
	_, _, _ = evm.Call(msg.From, *msg.To, msg.Data, 30_000_000, common.U2560)
	diff, _ := evm.StateDB.Finalise(true, nil)
	return diff
}

// ProcessParentBlockHash stores the parent block hash in the history storage contract
// as per EIP-2935/7709.
func ProcessParentBlockHash(prevHash common.Hash, evm *vm.EVM) *bal.StateDiff {
	if tracer := evm.Config.Tracer; tracer != nil {
		onSystemCallStart(tracer, evm.GetVMContext())
		if tracer.OnSystemCallEnd != nil {
			defer tracer.OnSystemCallEnd()
		}
	}
	msg := &Message{
		From:      params.SystemAddress,
		GasLimit:  30_000_000,
		GasPrice:  common.Big0,
		GasFeeCap: common.Big0,
		GasTipCap: common.Big0,
		To:        &params.HistoryStorageAddress,
		Data:      prevHash.Bytes(),
	}
	evm.SetTxContext(NewEVMTxContext(msg))
	evm.StateDB.AddAddressToAccessList(params.HistoryStorageAddress)
	_, _, err := evm.Call(msg.From, *msg.To, msg.Data, 30_000_000, common.U2560)
	if err != nil {
		panic(err)
	}
	if evm.StateDB.AccessEvents() != nil {
		evm.StateDB.AccessEvents().Merge(evm.AccessEvents)
	}
	diff, _ := evm.StateDB.Finalise(true, nil)
	return diff
}

// ProcessWithdrawalQueue calls the EIP-7002 withdrawal queue contract.
// It returns the opaque request data returned by the contract.
func ProcessWithdrawalQueue(requests *[][]byte, evm *vm.EVM) error {
	return processRequestsSystemCall(requests, evm, 0x01, params.WithdrawalQueueAddress)
}

// ProcessConsolidationQueue calls the EIP-7251 consolidation queue contract.
// It returns the opaque request data returned by the contract.
func ProcessConsolidationQueue(requests *[][]byte, evm *vm.EVM) error {
	return processRequestsSystemCall(requests, evm, 0x02, params.ConsolidationQueueAddress)
}

func processRequestsSystemCall(requests *[][]byte, evm *vm.EVM, requestType byte, addr common.Address) error {
	if tracer := evm.Config.Tracer; tracer != nil {
		onSystemCallStart(tracer, evm.GetVMContext())
		if tracer.OnSystemCallEnd != nil {
			defer tracer.OnSystemCallEnd()
		}
	}
	msg := &Message{
		From:      params.SystemAddress,
		GasLimit:  30_000_000,
		GasPrice:  common.Big0,
		GasFeeCap: common.Big0,
		GasTipCap: common.Big0,
		To:        &addr,
	}
	evm.SetTxContext(NewEVMTxContext(msg))
	evm.StateDB.AddAddressToAccessList(addr)
	ret, _, err := evm.Call(msg.From, *msg.To, msg.Data, 30_000_000, common.U2560)
	evm.StateDB.Finalise(true, nil)
	if err != nil {
		return fmt.Errorf("system call failed to execute: %v", err)
	}
	if len(ret) == 0 {
		return nil // skip empty output
	}
	// Append prefixed requestsData to the requests list.
	requestsData := make([]byte, len(ret)+1)
	requestsData[0] = requestType
	copy(requestsData[1:], ret)
	*requests = append(*requests, requestsData)
	return nil
}

var depositTopic = common.HexToHash("0x649bbc62d0e31342afea4e5cd82d4049e7e1ee912fc0889aa790803be39038c5")

// ParseDepositLogs extracts the EIP-6110 deposit values from logs emitted by
// BeaconDepositContract.
func ParseDepositLogs(requests *[][]byte, logs []*types.Log, config *params.ChainConfig) error {
	deposits := make([]byte, 1) // note: first byte is 0x00 (== deposit request type)
	for _, log := range logs {
		if log.Address == config.DepositContractAddress && len(log.Topics) > 0 && log.Topics[0] == depositTopic {
			request, err := types.DepositLogToRequest(log.Data)
			if err != nil {
				return fmt.Errorf("unable to parse deposit data: %v", err)
			}
			deposits = append(deposits, request...)
		}
	}
	if len(deposits) > 1 {
		*requests = append(*requests, deposits)
	}
	return nil
}

func onSystemCallStart(tracer *tracing.Hooks, ctx *tracing.VMContext) {
	if tracer.OnSystemCallStartV2 != nil {
		tracer.OnSystemCallStartV2(ctx)
	} else if tracer.OnSystemCallStart != nil {
		tracer.OnSystemCallStart()
	}
}
