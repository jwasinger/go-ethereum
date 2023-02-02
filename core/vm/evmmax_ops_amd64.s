#include "textflag.h"

TEXT ·opAddModX(SB),NOSPLIT,$16-64
    // AX <- &pc
    MOVQ    pc+16(SP), AX
    // R10 <- &scope
    MOVQ    scope+32(SP), R10
    // R11 <- &(scope.Contract)
    MOVQ    0(R10), R11
    // R11 <- &scope.Contract[0]
    MOVQ    0(R11), R11

    // R12 <- *pc
    MOVQ AX, R12
    MOVQ 0(R12), R12

    // *pc += 3
    MOVQ (AX), R13
    ADDQ $3, R13
    ADDQ R13, (AX)

    // R15 <- &memory[0]
    MOVQ 8(R10), R15
    MOVQ 0(R15), R15

    // BX <- x_slot = &mem[48 * uint64(scope.Contract.Code[*pc + 2])]
    MOVBLZX 0x2(R11)(R12*1), BX
    IMULQ $48, BX
    ADDQ R15, BX

    // store out_slot to scratch1
    MOVBLZX 0x1(R11)(R12*1), CX
    IMULQ $48, CX
    ADDQ R15, CX
    MOVQ CX, 8(SP)

    // store y_slot to scratch2
    MOVBLZX 0x3(R11)(R12*1), CX
    IMULQ $48, CX
    ADDQ R15, CX
    MOVQ CX, 16(SP)

    /*
        Arithmetic part
    */

    // R8..R13 <- x
    MOVQ 0(BX), R8
    MOVQ 8(BX), R9
    MOVQ 16(BX), R10
    MOVQ 24(BX), R11
    MOVQ 32(BX), R12
    MOVQ 40(BX), R13

    // R8..R13 <- x + y
    ADDQ    0(CX), R8
    ADCQ    8(CX), R9
    ADCQ    16(CX), R10
    MOVQ    R8, R14
    ADCQ    24(CX), R11
    MOVQ    R9, R15
    ADCQ    32(CX), R12
    MOVQ    R10, AX
    ADCQ    40(CX), R13
    MOVQ    R11, BX

    SBBQ    CX, CX

    // DX <- modulus
    MOVQ    scope+32(SP), DX
    MOVQ    16(DX), DX
    MOVQ    0(DX), DX

    SUBQ    0(DX), R8
    SUBQ    8(DX), R8
    MOVQ    R12, BP
    SBBQ    16(DX), R10
    SBBQ    24(DX), R11
    SBBQ    32(DX), R12
    MOVQ    R13, SI
    SBBQ    40(DX), R13

    MOVQ    8(SP), DI
    CMOVQCS R14, R8
    CMOVQCS R15, R9
    CMOVQCS AX, R10
    MOVQ    R8, 0(DI)
    CMOVQCS BX, R11
    MOVQ    R9, 8(DI)
    CMOVQCS BP, R12
    MOVQ    R10, 16(DI)
    CMOVQCS SI, R13
    MOVQ    R11, 24(DI)
    MOVQ    R12, 32(DI)
    MOVQ    R13, 40(DI)
    RET
