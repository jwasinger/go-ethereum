#include "textflag.h"

TEXT ·opAddModX(SB),NOSPLIT,$0-64
    // hardcode element size to 384 bits.  TODO fix this
    MOVQ $48, R10

    // AX <- pc
    MOVQ    pc+8(SP), AX

    // R10 <- scope
    MOVQ    scope+24(SP), R10
    // R11 <- scope.Contract
    MOVQ    0(R10), R11
    // R11 <- scope.Contract.Code
    MOVQ    0(R11), R11

    /*
    // R15 <- uint64(scope.Contract.Code[*pc + 1])
    MOVBLZX 0x1(R10)(AX*1), R15
    // TODO ^ fixme
    */

    // for now, just access memory at offset 0
    MOVQ $0, R15

    // R12 <- &(scope.Memory[0])
    MOVQ 8(R10), R12
    MOVQ 0(R12), R12
    
    // *pc += 3
    ADDQ    $3, (AX)

    // setup return
	MOVQ    $0, 24(SP)
	XORPS   X0, X0
	MOVUPS  X0, 32(SP)
	MOVUPS  X0, 48(SP)
	MOVUPS  X0, 56(SP)
	MOVUPS  X0, 64(SP)
	RET
