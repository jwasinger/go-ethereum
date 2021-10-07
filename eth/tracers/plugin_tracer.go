package tracers

import (
	"encoding/json"
	"errors"
	"math/big"
	"plugin"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
)

type NewFunc = func() PluginAPI

type PluginAPI interface {
	Step(log StepLog, db vm.StateReader)
    Enter(frame PluginTracerFrame)
    Exit(frameResult PluginTracerFrameResult)
	Result() (json.RawMessage, error)
}

type StepLog struct {
    Op vm.Opcode
    PC uint
    Gas uint
    Cost uint
    Depth uint
    Refund uint
    Memory PluginMemoryWrapper
    Contract PluginContractWrapper
    Stack PluginStackWrapper
    Error error
}

type PluginTracerFrame struct {
	Type string
	From common.Address
	To common.Address
	Input []byte
	Gas uint64
	Value *big.Int
}

type PluginTracerContext struct {
    From common.Address
    To common.Address
    Input []byte
    Gas uint64
    GasPrice *big.Int
    Value *big.Int
    IntrinsicGas uint64
    Type string

	Output []byte
	Time time.Duration
	GasUsed uint64
}

type PluginTracerFrameResult {
	GasUsed uint
	Output []byte
	Error string
}

type PluginMemoryWrapper struct {
    memory *vm.Memory
}

func (mw *PluginMemoryWrapper) Slice(begin, end int64) []byte {
    if end == begin {
        return []byte{}
    }
    if end < begin || begin < 0 {
        log.Warn("Tracer accessed out of bound memory", "offset", begin, "end", end)
        return nil
    }
    if mw.memory.Len() < int(end) {
        log.Warn("Tracer accessed out of bound memory", "available", mw.memory.Len(), "offset", begin, "size", end-begin)
        return nil
    }
    return mw.memory.GetCopy(begin, end-begin)
}

func (mw *PluginMemoryWrapper) GetUint(addr uint64) *big.Int {
    if mw.memory.Len() < int(addr)+32 || addr < 0 {
        log.Warn("Tracer accessed out of bound memory", "available", mw.memory.Len(), "offset", addr, "size", 32)
        return new(big.Int)
    }
    return new(big.Int).SetBytes(mw.memory.GetPtr(addr, 32))
}

type PluginStackWrapper struct {
	stack *vm.Stack
}

func (s *PluginStackWrapper) Peek(idx int) *big.Int {
    if len(sw.stack.Data()) <= idx || idx < 0 {
        log.Warn("Tracer accessed out of bound stack", "size", len(sw.stack.Data()), "index", idx)
        return new(big.Int)
    }
    return sw.stack.Back(idx).ToBig()
}

type PluginContract struct {
    contract *vm.Contract
}

type PluginTracer struct {
	plugin *plugin.Plugin
	tracer PluginAPI
}

func NewPluginTracer(path string) (*PluginTracer, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, err
	}
	newSym, err := p.Lookup("New")
	if err != nil {
		return nil, err
	}
	newF, ok := newSym.(NewFunc)
	if !ok {
		return nil, errors.New("plugin has invalid new signature")
	}

	t := newF()

	return &PluginTracer{plugin: p, tracer: t}, nil
}

func (t *PluginTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
    t.ctx = Context{
        From: from,
        To: to,
        Input: input, // TODO copy here?
        Gas: gas,
        GasPrice: env.TxContext.GasPrice,
        Value: value,
    }
    if create {
        t.ctx.Type = "CREATE"
    }
    t.db = env.db
    t.activePrecompiles = vm.ActivePrecompiles(rules)

    // Compute intrinsic gas                                                                         
    isHomestead := env.ChainConfig().IsHomestead(env.Context.BlockNumber)
    isIstanbul := env.ChainConfig().IsIstanbul(env.Context.BlockNumber)
    intrinsicGas, err := core.IntrinsicGas(input, nil, jst.ctx["type"] == "CREATE", isHomestead, isIstanbul)
    if err != nil {
        // TODO why failure is silent here?
        return
    }

    t.ctxt.IntrinsictGas = intrinsicGas
}

func (t *PluginTracer) CaptureState(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
    if !t.traceSteps {
        return
    }
    if t.err != nil {
        return
    }
    // If tracing was interrupted, set the error and stop
    if atomic.LoadUint32(&t.interrupt) > 0 {
        t.err = t.reason
        env.Cancel()
        return
    }

    memory := PluginMemoryWrapper{scope.Memory}
    contract := PluginContractWrapper{scope.Contract}
    stack := PluginStackWrapper{scope.Stack}

    log := StepLog{
        Op: op,
        PC: uint(pc),
        Gas: uint(gas),
        Cost: uint(cost),
        Depth: uint(depth),
        Refund: uint(env.StateDB.GetRefund()),
        Error: nil,
    }

/*
	// TODO wat do here

    t.errorValue := nil
    if err != nil {
        t.errorValue = new(string)
        *t.errorValue = err.Error()
    }
*/

    t.tracer.Step(log, env.StateDB)
}

func (t *PluginTracer) CaptureFault(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, _ *vm.ScopeContext, depth int, err error) {
	if t.Error != nil {
		return
	}
	/* TODO Wat do here?  err vs errVal??? */
}

func (t *PluginTracer) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) {
	t.Ctx.Output = output // TODO copy here?
	t.Ctx.Gasused = gasUsed
	t.Ctx.time = t

	if err != nil {
		t.Ctx.Error = err
	}
}

func (t *PluginTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {

	if !t.traceCallFrames {
		return
	}
/*
	// TODO wat do here
    if jst.err != nil {
        return 
    }  
*/
    // If tracing was interrupted, set the error and stop 
    if atomic.LoadUint32(&jst.interrupt) > 0 {
        jst.err = jst.reason
        return
    }

	frame := PluginTracerFrame{
		Type: typ.String(),
        From: from,
        To: to,
        Input: common.CopyBytes(input),
        Gas: uint(gas),
        Value: nil,
	}

    if value != nil {
        frame.Value = new(big.Int).SetInt(value)
    }
    t.tracer.Enter(frame)
}

func (t *PluginTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
    if !t.traceCallFrames {
        return
    }
    // If tracing was interrupted, set the error and stop
    if atomic.LoadUint32(&jst.interrupt) > 0 {
        jst.err = jst.reason
        return
    }

    frameResult := PluginTracerFrameResult{
	    Output: common.CopyBytes(output),
	    GasUsed: uint(gasUsed),
	    Error: err,
    }

	t.tracer.Exit(frameResult)
}

func (t *PluginTracer) GetResult() (json.RawMessage, error) {
	result, err = t.tracer.Result()
    if err != nil {
        return err
    }

    return result
}
