#include "textflag.h"

TEXT ·opAddModX(SB),NOSPLIT,$0-64
    // *pc += 3
    MOVQ    pc+8(SP), AX
    ADDQ    $3, (AX)

    // setup return
	MOVQ    $0, 24(SP)
	XORPS   X0, X0
	MOVUPS  X0, 32(SP)
	MOVUPS  X0, 48(SP)
	MOVUPS  X0, 56(SP)
	MOVUPS  X0, 64(SP)
	RET
