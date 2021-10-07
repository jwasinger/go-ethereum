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
	Step(op vm.OpCode)
    Enter()
    Exit()
	Result() (json.RawMessage, error)
}

type StepLog struct {
    Pc uint
    Gas uint
    Cost uint
    Depth uint
    Refund uint
    Memory PluginMemoryWrapper
    Contract PluginContractWrapper
    Stack PluginStackWrapper
}

// transaction context
type PluginContext struct {
    From common.Address
    To common.Address
    Input []byte
    Gas uint64
    GasPrice *big.Int
    Value *big.Int
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
	Caller common.Address
	Address common.Address
	Value *big.Int
	Input []byte
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
	log.Info("PluginTracer.CaptureStart")
	t.db = env.db
}

func (t *PluginTracer) CaptureState(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, _ *vm.ScopeContext, rData []byte, depth int, err error) {
	t.tracer.Step(op)
}

func (t *PluginTracer) CaptureFault(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, _ *vm.ScopeContext, depth int, err error) {
}

func (t *PluginTracer) CaptureEnd(output []byte, gasUsed uint64, t_ time.Duration, err error) {
}

func (t *PluginTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

func (t *PluginTracer) CaptureExit(output []byte, gasUsed uint64, err error) {}

func (t *PluginTracer) GetResult() (json.RawMessage, error) {
	return t.tracer.Result()
}
