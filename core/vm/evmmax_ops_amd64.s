#include "textflag.h"

TEXT ·opAddModX(SB),NOSPLIT,$0-24
	MOVQ    $0, 24(SP)
	XORPS   X0, X0
	MOVUPS  X0, 32(SP)
	MOVUPS  X0, 48(SP)
	RET
