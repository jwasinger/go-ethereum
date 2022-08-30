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

package native

import (
	"encoding/json"
	"errors"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
)

func init() {
	register("precompiles", newPrecompiledCallTracer)
}

type precompiledCallFrame struct {
	Type    string `json:"type"`
	From    string `json:"from"`
	To      string `json:"to,omitempty"`
	Value   string `json:"value,omitempty"`
	Gas     string `json:"gas"`
	GasUsed string `json:"gasUsed"`
	Input   string `json:"input"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

type precompiledCallTracer struct {
	env             *vm.EVM
	callstack       []callFrame
	interrupt       uint32 // Atomic flag to signal execution interruption
	reason          error  // Textual reason for the interruption
	isInPrecompiled bool
}

var precompiledContractsBerlin = map[common.Address]struct{}{
	common.BytesToAddress([]byte{1}): {},
	common.BytesToAddress([]byte{2}): {},
	common.BytesToAddress([]byte{3}): {},
	common.BytesToAddress([]byte{4}): {},
	common.BytesToAddress([]byte{5}): {},
	common.BytesToAddress([]byte{6}): {},
	common.BytesToAddress([]byte{7}): {},
	common.BytesToAddress([]byte{8}): {},
	common.BytesToAddress([]byte{9}): {},
}

// newCallTracer returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func newPrecompiledCallTracer(ctx *tracers.Context) tracers.Tracer {
	// First callframe contains tx context info
	// and is populated on start and end.
	return &precompiledCallTracer{callstack: make([]callFrame, 1)}
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *precompiledCallTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.env = env
	if !create {
		if _, isPrecompiled := precompiledContractsBerlin[to]; isPrecompiled {
			t.callstack = append(t.callstack, callFrame{
				Type:  "CALL",
				From:  addrToHex(from),
				To:    addrToHex(to),
				Input: bytesToHex(input),
				Gas:   uintToHex(gas),
				Value: bigToHex(value),
			})
			t.isInPrecompiled = true
		}
	}
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *precompiledCallTracer) CaptureEnd(output []byte, gasUsed uint64, _ time.Duration, err error) {
	if t.isInPrecompiled {
		t.callstack[len(t.callstack)-1].GasUsed = uintToHex(gasUsed)
		if err != nil {
			t.callstack[len(t.callstack)-1].Error = err.Error()
			if err.Error() == "execution reverted" && len(output) > 0 {
				t.callstack[len(t.callstack)-1].Output = bytesToHex(output)
			}
		} else {
			t.callstack[len(t.callstack)-1].Output = bytesToHex(output)
		}

		t.isInPrecompiled = false
	}
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *precompiledCallTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
}

// CaptureFault implements the EVMLogger interface to trace an execution fault.
func (t *precompiledCallTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, _ *vm.ScopeContext, depth int, err error) {
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *precompiledCallTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	// Skip if tracing was interrupted
	if atomic.LoadUint32(&t.interrupt) > 0 {
		t.env.Cancel()
		return
	}

	if _, isPrecompiled := precompiledContractsBerlin[to]; isPrecompiled {
		if t.isInPrecompiled {
			panic("should never occur: cannot call a precompile from a precompile")
		}

		call := callFrame{
			Type:  typ.String(),
			From:  addrToHex(from),
			To:    addrToHex(to),
			Input: bytesToHex(input),
			Gas:   uintToHex(gas),
			Value: bigToHex(value),
		}
		t.callstack = append(t.callstack, call)
		t.isInPrecompiled = true
	}
}

// TODO check that proper call origin is preserved in delegatecall

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *precompiledCallTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
	if !t.isInPrecompiled {
		return
	}
	t.isInPrecompiled = false

	call := t.callstack[len(t.callstack)-1]
	if err == nil {
		call.Output = bytesToHex(output)
	} else {
		call.Error = err.Error()
	}
}

func (*precompiledCallTracer) CaptureTxStart(gasLimit uint64) {}

func (*precompiledCallTracer) CaptureTxEnd(restGas uint64) {}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *precompiledCallTracer) GetResult() (json.RawMessage, error) {
	if len(t.callstack) != 1 {
		return nil, errors.New("incorrect number of top-level calls")
	}
	res, err := json.Marshal(t.callstack)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(res), t.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *precompiledCallTracer) Stop(err error) {
	t.reason = err
	atomic.StoreUint32(&t.interrupt, 1)
}
