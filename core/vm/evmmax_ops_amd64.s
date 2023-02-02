#include "textflag.h"

TEXT ·opAddModX(SB),NOSPLIT,$16-64
	// pc += 3
    MOVQ    pc+16(SP), AX
    MOVQ (AX), R13
    ADDQ $3, R13
    ADDQ R13, (AX)

    // return nil, nil
    MOVQ $0, 56(SP)
    MOVQ $0, 64(SP)
    MOVQ $0, 72(SP)
    MOVQ $0, 80(SP)
    MOVQ $0, 88(SP)
    RET
