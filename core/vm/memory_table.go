// Copyright 2017 The go-ethereum Authors
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

package vm

func memoryKeccak256(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(1))
	return 0, evmMemUsed, overflow
}

func memoryCallDataCopy(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(2))
	return 0, evmMemUsed, overflow
}

func memoryReturnDataCopy(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(2))
	return 0, evmMemUsed, overflow
}

func memoryCodeCopy(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(2))
	return 0, evmMemUsed, overflow
}

func memoryExtCodeCopy(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(1), stack.Back(3))
	return 0, evmMemUsed, overflow
}

func memoryMLoad(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64WithUint(stack.Back(0), 32)
	return 0, evmMemUsed, overflow
}

func memoryMStore8(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64WithUint(stack.Back(0), 1)
	return 0, evmMemUsed, overflow
}

func memoryMStore(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64WithUint(stack.Back(0), 32)
	return 0, evmMemUsed, overflow
}

func memoryMcopy(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	mStart := stack.Back(0) // stack[0]: dest
	if stack.Back(1).Gt(mStart) {
		mStart = stack.Back(1) // stack[1]: source
	}
	evmMemUsed, overflow := calcMemSize64(mStart, stack.Back(2)) // stack[2]: length
	return 0, evmMemUsed, overflow
}

func memoryCreate(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(1), stack.Back(2))
	return 0, evmMemUsed, overflow
}

func memoryCreate2(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(1), stack.Back(2))
	return 0, evmMemUsed, overflow
}

func memoryCall(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	x, overflow := calcMemSize64(stack.Back(5), stack.Back(6))
	if overflow {
		return 0, 0, true
	}
	y, overflow := calcMemSize64(stack.Back(3), stack.Back(4))
	if overflow {
		return 0, 0, true
	}
	if x > y {
		return 0, x, false
	}
	return 0, y, false
}
func memoryDelegateCall(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	x, overflow := calcMemSize64(stack.Back(4), stack.Back(5))
	if overflow {
		return 0, 0, true
	}
	y, overflow := calcMemSize64(stack.Back(2), stack.Back(3))
	if overflow {
		return 0, 0, true
	}
	if x > y {
		return 0, x, false
	}
	return 0, y, false
}

func memoryStaticCall(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	x, overflow := calcMemSize64(stack.Back(4), stack.Back(5))
	if overflow {
		return 0, 0, true
	}
	y, overflow := calcMemSize64(stack.Back(2), stack.Back(3))
	if overflow {
		return 0, 0, true
	}
	if x > y {
		return 0, x, false
	}
	return 0, y, false
}

func memoryReturn(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(1))
	return 0, evmMemUsed, overflow
}

func memoryRevert(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(1))
	return 0, evmMemUsed, overflow
}

func memoryLog(_ *ScopeContext, stack *Stack) (uint64, uint64, bool) {
	evmMemUsed, overflow := calcMemSize64(stack.Back(0), stack.Back(1))
	return 0, evmMemUsed, overflow
}
