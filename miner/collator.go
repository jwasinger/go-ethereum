// Copyright 2021 The go-ethereum Authors
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

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"errors"
	//	"math"
	//"math/big"
	"os"
	"plugin"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	//    "github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/params"
	"github.com/naoina/toml"
)

type CollatorAPI interface {
	Version() string
	Service() interface{}
}

// Pool is an interface to the transaction pool
type Pool interface {
	Pending(bool) (map[common.Address]types.Transactions, error)
	Locals() []common.Address
}

/*
	BlockState represents an under-construction block.  An instance of
	BlockState is passed to CollateBlock where it can be filled with transactions
	via BlockState.AddTransaction() and submitted for sealing via
	BlockState.Commit().

	Operations on a single BlockState instance are not threadsafe.  However,
	instances can be copied with BlockState.Copy().
*/
type BlockState interface {
	/*
		adds a single transaction to the blockState.  Returned errors include ..,..,..
		which signify that the transaction was invalid for the current EVM/chain state.

		ErrRecommit signals that the recommit timer has elapsed.
		ErrNewHead signals that the client has received a new canonical chain head.
		All subsequent calls to AddTransaction fail if either newHead or the recommit timer
		have occured.

		If the recommit interval has elapsed, the BlockState can still be committed to the sealer.
	*/
	AddTransaction(tx *types.Transaction) (error, *types.Receipt)

	/*
		removes a number of transactions from the block resetting the state to what
		it was before the transactions were added.  If count is greater than the number
		of transactions in the block,  returns
	*/
	RevertTransactions(count uint) error

	/*
		returns true if the Block has been made the current sealing block.
		returns false if the newHead interrupt has been triggered.
		can also return false if the BlockState is no longer valid (the call to CollateBlock
		which the original instance was passed has returned).
	*/
	Commit() bool
	Copy() BlockState
	State() vm.StateReader
	Signer() types.Signer
	Header() *types.Header
	/*
		the account which will receive the block reward.
	*/
	Etherbase() common.Address
}

const (
	InterruptNone int = iota
	InterruptResubmit
	InterruptNewHead
)

/*
InterruptContext allows for active polling to detect if a new canonical chain
head was received or recommit timer elapse occured.
*/
type InterruptContext interface {
	InterruptState() int
}

type Collator interface {
	/*
		the main entrypoint for constructing a block for sealing. An empty block
		bs, is provided which can be modified/copied and committed to the sealer
		for the duration of the call to CollateBlock.
	*/
	CollateBlock(bs BlockState, ctx InterruptContext)
	/*
		Called when the client is started after the miner worker is created.
	*/
	Start(pool Pool)
	/*
		Called when the client is closing.
	*/
	Close()
}

var (
	ErrInterruptRecommit = errors.New("interrupt: recommit timer elapsed")
	ErrInterruptNewHead  = errors.New("interrupt: client received new canon chain head")

	// errors which indicate that a given transaction cannot be
	// added at a given block or chain configuration.
	ErrGasLimitReached    = errors.New("gas limit reached")
	ErrNonceTooLow        = errors.New("tx nonce too low")
	ErrNonceTooHigh       = errors.New("tx nonce too high")
	ErrTxTypeNotSupported = errors.New("tx type not supported")
	ErrGasFeeCapTooLow    = errors.New("gas fee cap too low")
	// error which encompasses all other reasons a given transaction
	// could not be added to the block.
	ErrStrange = errors.New("strange error")
)

/*
	Loads a collator plugin and configuration (toml) from disk.
	Expects the plugin to export a method named PluginConstructor
	which has a signature:
		func(config *map[string]interface{}) (Collator, CollatorAPI, error)

	returns the result of calling the plugin constructor for the given
	toml config (which is nil if a custom config filepath is not passed
	via --minercollator.configfilepath)
*/
func LoadCollator(filepath string, configPath string) (Collator, CollatorAPI, error) {
	p, err := plugin.Open(filepath)
	if err != nil {
		return nil, nil, err
	}

	v, err := p.Lookup("PluginConstructor")
	if err != nil {
		return nil, nil, errors.New("Symbol 'APIExport' not found")
	}

	pluginConstructor, ok := v.(func(config *map[string]interface{}) (Collator, CollatorAPI, error))
	if !ok {
		return nil, nil, errors.New("Expected symbol 'API' to be of type 'CollatorAPI")
	}

	f, err := os.Open(configPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	config := make(map[string]interface{})
	if err := toml.NewDecoder(f).Decode(&config); err != nil {
		return nil, nil, err
	}

	collator, collatorAPI, err := pluginConstructor(&config)
	if err != nil {
		return nil, nil, err
	}

	return collator, collatorAPI, nil
}

const (
	interruptNotHandled int32 = 0
	interruptIsHandled  int32 = 1
)

type blockState struct {
	worker     *worker
	env        *environment
	start      time.Time
	logs       []*types.Log
	shouldSeal bool
	snapshots  []int
	committed  bool

	// shared values between multiple copies of a blockState

	interrupt *int32
	// mutex to make sure only one blockState is calling commit at a given time
	commitMu *sync.Mutex
	// this makes sure multiple copies of a blockState can only trigger
	// adjustment of the recommit interval once
	interruptHandled *int32
	// prevents calls to worker.commit (with a given blockState) after
	// CollateBlock call on that blockState returns. examined in commit
	// when commitMu is held.  modified right after CollateBlock returns
	done *bool
	// calling Commit() copies the value of env to this value
	// and forwards it to the sealer via worker.commit() if shouldSeal is true
	resultEnv *environment
}

type interruptContext struct {
	interrupt *int32
}

func (ctx *interruptContext) InterruptState() int {
	if ctx.interrupt == nil {
		return InterruptNone
	}

	switch atomic.LoadInt32(ctx.interrupt) {
	case commitInterruptResubmit:
		return InterruptResubmit
	case commitInterruptNewHead:
		return InterruptNewHead
	default:
		return InterruptNone
	}
}

func (bs *blockState) Etherbase() common.Address {
	return bs.env.etherbase
}

func (bs *blockState) Header() *types.Header {
	return types.CopyHeader(bs.env.header)
}

func (bs *blockState) AddTransaction(tx *types.Transaction) (error, *types.Receipt) {
	if bs.interrupt != nil && atomic.LoadInt32(bs.interrupt) != commitInterruptNone {
		if atomic.CompareAndSwapInt32(bs.interruptHandled, interruptNotHandled, interruptIsHandled) && atomic.LoadInt32(bs.interrupt) == commitInterruptResubmit {
			var ratio float64 = 0.1
			bs.worker.resubmitAdjustCh <- &intervalAdjust{
				ratio: ratio,
				inc:   true,
			}
			return ErrInterruptRecommit, nil
		} else {
			return ErrInterruptNewHead, nil
		}
	}

	if bs.env.gasPool.Gas() < params.TxGas {
		return ErrGasLimitReached, nil
	}

	// Check whether the tx is replay protected. If we're not in the EIP155 hf
	// phase, start ignoring the sender until we do.
	if tx.Protected() && !bs.worker.chainConfig.IsEIP155(bs.env.header.Number) {
		return ErrTxTypeNotSupported, nil
	}

	// TODO can this error also be returned by commitTransaction below?
	_, err := tx.EffectiveGasTip(bs.env.header.BaseFee)
	if err != nil {
		return ErrGasFeeCapTooLow, nil
	}

	bs.env.state.Prepare(tx.Hash(), bs.env.tcount)
	txLogs, err := bs.worker.commitTransaction(bs.env, tx, bs.env.etherbase)
	if err != nil {
		switch {
		case errors.Is(err, core.ErrGasLimitReached):
			// this should never be reached.
			// should be caught above
			return ErrGasLimitReached, nil
		case errors.Is(err, core.ErrNonceTooLow):
			return ErrNonceTooLow, nil
		case errors.Is(err, core.ErrNonceTooHigh):
			return ErrNonceTooHigh, nil
		case errors.Is(err, core.ErrTxTypeNotSupported):
			// TODO check that this unspported tx type is the same as the one caught above
			return ErrTxTypeNotSupported, nil
		default:
			return ErrStrange, nil
		}
	} else {
		bs.logs = append(bs.logs, txLogs...)
		bs.env.tcount++
	}

	return nil, bs.env.receipts[len(bs.env.receipts)-1]
}

func (bs *blockState) State() vm.StateReader {
	return bs.env.state
}

func (bs *blockState) Signer() types.Signer {
	return bs.env.signer
}

func (bs *blockState) Commit() bool {
	if bs.interrupt != nil && atomic.LoadInt32(bs.interrupt) != commitInterruptNone {
		if atomic.CompareAndSwapInt32(bs.interruptHandled, interruptNotHandled, interruptIsHandled) && atomic.LoadInt32(bs.interrupt) == commitInterruptResubmit {
			// Notify resubmit loop to increase resubmitting interval due to too frequent commits.
			var ratio float64 = 0.1
			bs.worker.resubmitAdjustCh <- &intervalAdjust{
				ratio: ratio,
				inc:   true,
			}
		}
		return false
	}

	bs.commitMu.Lock()
	defer bs.commitMu.Unlock()
	if *bs.done {
		return false
	}
	if bs.shouldSeal {
		bs.worker.commit(bs.env.copy(), bs.worker.fullTaskHook, true, bs.start)
	}
	bs.resultEnv = bs.env
	bs.committed = true
	return true
}

var (
	ErrTooManyTxs = errors.New("tried to revert more txs than exist in BlockState")
	ErrZeroTxs    = errors.New("tried to revert 0 transactions")
)

func (bs *blockState) RevertTransactions(count uint) error {
	if int(count) > len(bs.snapshots) {
		return ErrTooManyTxs
	} else if count == 0 {
		return ErrZeroTxs
	}
	bs.env.state.RevertToSnapshot(bs.snapshots[len(bs.snapshots)-int(count)])
	bs.snapshots = bs.snapshots[:len(bs.snapshots)-int(count)]
	return nil
}

func (bs *blockState) Copy() BlockState {
	return &blockState{
		worker:           bs.worker,
		env:              bs.env.copy(),
		start:            bs.start,
		logs:             copyLogs(bs.logs),
		interrupt:        bs.interrupt,
		commitMu:         bs.commitMu,
		interruptHandled: bs.interruptHandled,
		done:             bs.done,
		committed:        bs.committed,
		shouldSeal:       bs.shouldSeal,
	}
}
