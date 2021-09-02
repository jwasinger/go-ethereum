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

type BlockState interface {
	AddTransaction(tx *types.Transaction) (error, *types.Receipt)
	Commit() bool
	Copy() BlockState
	Signer() types.Signer
	Header() *types.Header
}

type Collator interface {
	CollateBlock(bs BlockState, pool Pool, state vm.StateReader)
	Start()
	Close()
}

var (
	ErrInterrupt          = errors.New("interrupt triggered")
	ErrGasLimitReached    = errors.New("gas limit reached")
	ErrNonceTooLow        = errors.New("tx nonce too low")
	ErrNonceTooHigh       = errors.New("tx nonce too high")
	ErrTxTypeNotSupported = errors.New("tx type not supported")
	ErrStrange            = errors.New("strange error")
	ErrGasFeeCapTooLow    = errors.New("gas fee cap too low")
)

type CollatorPluginConstructorFunc func(config *map[string]interface{}) (*Collator, *CollatorAPI, error)

func LoadCollator(filepath string, configPath string) (*Collator, *CollatorAPI, error) {
	p, err := plugin.Open(filepath)
	if err != nil {
		return nil, nil, err
	}

	v, err := p.Lookup("PluginConstructor")
	if err != nil {
		return nil, nil, errors.New("Symbol 'APIExport' not found")
	}
	pluginConstructor, ok := v.(CollatorPluginConstructorFunc)
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

func (bs *blockState) Header() *types.Header {
	return types.CopyHeader(bs.env.header)
}

func (bs *blockState) AddTransaction(tx *types.Transaction) (error, *types.Receipt) {
	if bs.interrupt != nil && atomic.LoadInt32(bs.interrupt) != commitInterruptNone {
		if atomic.CompareAndSwapInt32(bs.interruptHandled, interruptNotHandled, interruptIsHandled) && atomic.LoadInt32(bs.interrupt) == commitInterruptResubmit {
			// Notify resubmit loop to increase resubmitting interval due to too frequent commits.
			// TODO figure out a better heuristic for the adjust ratio here
			// the gasRemaining/gasLimit is not a good proxy for all collators
			// where the
			gasLimit := bs.env.header.GasLimit
			ratio := float64(gasLimit-bs.env.gasPool.Gas()) / float64(gasLimit)
			if ratio < 0.1 {
				ratio = 0.1
			}
			bs.worker.resubmitAdjustCh <- &intervalAdjust{
				ratio: ratio,
				inc:   true,
			}
		}
		return ErrInterrupt, nil
	}

	if bs.env.gasPool.Gas() < params.TxGas {
		return ErrGasLimitReached, nil
	}

	// from, _ := types.Sender(bs.env.signer, tx)
	// TODO use this and add log messages back?

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

func (bs *blockState) Signer() types.Signer {
	return bs.env.signer
}

func (bs *blockState) Commit() bool {
	if bs.interrupt != nil && atomic.LoadInt32(bs.interrupt) != commitInterruptNone {
		if atomic.CompareAndSwapInt32(bs.interruptHandled, interruptNotHandled, interruptIsHandled) && atomic.LoadInt32(bs.interrupt) == commitInterruptResubmit {
			// Notify resubmit loop to increase resubmitting interval due to too frequent commits.
			gasLimit := bs.env.header.GasLimit
			// TODO figure out a better heuristic for the adjust ratio here
			// the gasRemaining/gasLimit is not a good proxy for all collators
			// where the
			ratio := float64(gasLimit-bs.env.gasPool.Gas()) / float64(gasLimit)
			if ratio < 0.1 {
				ratio = 0.1
			}
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
	return true
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
	}
}
