package evmmax

import (
	"encoding/binary"
	"math/bits"
)

// adapted from gnark implementation of CIOS algorithm
func MulMont384Fallback(state *ModState, zBytes, xBytes, yBytes []byte) {
	x := make([]uint64, 6)
	y := make([]uint64, 6)
	mod := make([]uint64, 6)
	var z [6]uint64

	var t [7]uint64
	var D uint64
	var m, C uint64

	// hints to compiler
	_ = xBytes[47]
	_ = yBytes[47]
	_ = zBytes[47]
	_ = state.Mod[47]

	x[0] = binary.LittleEndian.Uint64(xBytes[0:8])
	x[1] = binary.LittleEndian.Uint64(xBytes[8:16])
	x[2] = binary.LittleEndian.Uint64(xBytes[16:24])
	x[3] = binary.LittleEndian.Uint64(xBytes[24:32])
	x[4] = binary.LittleEndian.Uint64(xBytes[32:40])
	x[5] = binary.LittleEndian.Uint64(xBytes[40:48])

	y[0] = binary.LittleEndian.Uint64(yBytes[0:8])
	y[1] = binary.LittleEndian.Uint64(yBytes[8:16])
	y[2] = binary.LittleEndian.Uint64(yBytes[16:24])
	y[3] = binary.LittleEndian.Uint64(yBytes[24:32])
	y[4] = binary.LittleEndian.Uint64(yBytes[32:40])
	y[5] = binary.LittleEndian.Uint64(yBytes[40:48])

	mod[0] = binary.LittleEndian.Uint64(state.Mod[0:8])
	mod[1] = binary.LittleEndian.Uint64(state.Mod[8:16])
	mod[2] = binary.LittleEndian.Uint64(state.Mod[16:24])
	mod[3] = binary.LittleEndian.Uint64(state.Mod[24:32])
	mod[4] = binary.LittleEndian.Uint64(state.Mod[32:40])
	mod[5] = binary.LittleEndian.Uint64(state.Mod[40:48])

	for j := 0; j < 6; j++ {
		// t <- x[j] * y
		C, t[0] = madd1(x[j], y[0], t[0])
		C, t[1] = madd2(x[j], y[1], t[1], C)
		C, t[2] = madd2(x[j], y[2], t[2], C)
		C, t[3] = madd2(x[j], y[3], t[3], C)
		C, t[4] = madd2(x[j], y[4], t[4], C)
		C, t[5] = madd2(x[j], y[5], t[5], C)
		t[6], D = bits.Add64(t[6], C, 0)

		// m = ((x * y) % (1<<64)) * pow(-mod, -1, 1<<64)
		m = t[0] * state.modInv

		// t <- (t + t * mod * m) / (1<<64)
		C = madd0(m, mod[0], t[0])
		C, t[0] = madd2(m, mod[1], t[1], C)
		C, t[1] = madd2(m, mod[2], t[2], C)
		C, t[2] = madd2(m, mod[3], t[3], C)
		C, t[3] = madd2(m, mod[4], t[4], C)
		C, t[4] = madd2(m, mod[5], t[5], C)
		t[5], C = bits.Add64(t[6], C, 0)
		t[6], _ = bits.Add64(0, D, C)
	}

	// t == ((x * y) + (x * y) * mod * pow(-mod, -1, 1<<384)) / (1<<384)
	// 0 < t < 2 * mod

	// conditional subtraction to reduce t by mod if necessary
	z[0], D = bits.Sub64(t[0], mod[0], 0)
	z[1], D = bits.Sub64(t[1], mod[1], D)
	z[2], D = bits.Sub64(t[2], mod[2], D)
	z[3], D = bits.Sub64(t[3], mod[3], D)
	z[4], D = bits.Sub64(t[4], mod[4], D)
	z[5], D = bits.Sub64(t[5], mod[5], D)

	var src []uint64
	if D != 0 && t[6] == 0 {
		// final subtraction was unecessary
		src = t[:6]
	} else {
		src = z[:]
	}
	binary.LittleEndian.PutUint64(zBytes[0:8], src[0])
	binary.LittleEndian.PutUint64(zBytes[8:16], src[1])
	binary.LittleEndian.PutUint64(zBytes[16:24], src[2])
	binary.LittleEndian.PutUint64(zBytes[24:32], src[3])
	binary.LittleEndian.PutUint64(zBytes[32:40], src[4])
	binary.LittleEndian.PutUint64(zBytes[40:48], src[5])
}

func AddMod384Fallback(m *ModState, zBytes, xBytes, yBytes []byte) {
	x := make([]uint64, 6)
	y := make([]uint64, 6)
	mod := make([]uint64, 6)
	var z [6]uint64

	_ = xBytes[47]
	_ = yBytes[47]
	_ = zBytes[47]
	_ = m.Mod[47]

	x[0] = binary.LittleEndian.Uint64(xBytes[0:8])
	x[1] = binary.LittleEndian.Uint64(xBytes[8:16])
	x[2] = binary.LittleEndian.Uint64(xBytes[16:24])
	x[3] = binary.LittleEndian.Uint64(xBytes[24:32])
	x[4] = binary.LittleEndian.Uint64(xBytes[32:40])
	x[5] = binary.LittleEndian.Uint64(xBytes[40:48])

	y[0] = binary.LittleEndian.Uint64(yBytes[0:8])
	y[1] = binary.LittleEndian.Uint64(yBytes[8:16])
	y[2] = binary.LittleEndian.Uint64(yBytes[16:24])
	y[3] = binary.LittleEndian.Uint64(yBytes[24:32])
	y[4] = binary.LittleEndian.Uint64(yBytes[32:40])
	y[5] = binary.LittleEndian.Uint64(yBytes[40:48])

	mod[0] = binary.LittleEndian.Uint64(m.Mod[0:8])
	mod[1] = binary.LittleEndian.Uint64(m.Mod[8:16])
	mod[2] = binary.LittleEndian.Uint64(m.Mod[16:24])
	mod[3] = binary.LittleEndian.Uint64(m.Mod[24:32])
	mod[4] = binary.LittleEndian.Uint64(m.Mod[32:40])
	mod[5] = binary.LittleEndian.Uint64(m.Mod[40:48])

	var c uint64 = 0
	var c1 uint64 = 0
	tmp := [6]uint64{0, 0, 0, 0, 0, 0}

	tmp[0], c = bits.Add64(x[0], y[0], c)
	tmp[1], c = bits.Add64(x[1], y[1], c)
	tmp[2], c = bits.Add64(x[2], y[2], c)
	tmp[3], c = bits.Add64(x[3], y[3], c)
	tmp[4], c = bits.Add64(x[4], y[4], c)
	tmp[5], c = bits.Add64(x[5], y[5], c)

	z[0], c1 = bits.Sub64(tmp[0], mod[0], c1)
	z[1], c1 = bits.Sub64(tmp[1], mod[1], c1)
	z[2], c1 = bits.Sub64(tmp[2], mod[2], c1)
	z[3], c1 = bits.Sub64(tmp[3], mod[3], c1)
	z[4], c1 = bits.Sub64(tmp[4], mod[4], c1)
	z[5], c1 = bits.Sub64(tmp[5], mod[5], c1)

	var src []uint64
	if c == 0 && c1 != 0 {
		// final sub was unnecessary
		src = tmp[:]
	} else {
		src = z[:]
	}

	binary.LittleEndian.PutUint64(zBytes[0:8], src[0])
	binary.LittleEndian.PutUint64(zBytes[8:16], src[1])
	binary.LittleEndian.PutUint64(zBytes[16:24], src[2])
	binary.LittleEndian.PutUint64(zBytes[24:32], src[3])
	binary.LittleEndian.PutUint64(zBytes[32:40], src[4])
	binary.LittleEndian.PutUint64(zBytes[40:48], src[5])
}

func SubMod384Fallback(m *ModState, zBytes, xBytes, yBytes []byte) {
	x := make([]uint64, 6)
	y := make([]uint64, 6)
	mod := make([]uint64, 6)
	var z [6]uint64

	_ = xBytes[47]
	_ = yBytes[47]
	_ = zBytes[47]
	_ = m.Mod[47]

	x[0] = binary.LittleEndian.Uint64(xBytes[0:8])
	x[1] = binary.LittleEndian.Uint64(xBytes[8:16])
	x[2] = binary.LittleEndian.Uint64(xBytes[16:24])
	x[3] = binary.LittleEndian.Uint64(xBytes[24:32])
	x[4] = binary.LittleEndian.Uint64(xBytes[32:40])
	x[5] = binary.LittleEndian.Uint64(xBytes[40:48])

	y[0] = binary.LittleEndian.Uint64(yBytes[0:8])
	y[1] = binary.LittleEndian.Uint64(yBytes[8:16])
	y[2] = binary.LittleEndian.Uint64(yBytes[16:24])
	y[3] = binary.LittleEndian.Uint64(yBytes[24:32])
	y[4] = binary.LittleEndian.Uint64(yBytes[32:40])
	y[5] = binary.LittleEndian.Uint64(yBytes[40:48])

	mod[0] = binary.LittleEndian.Uint64(m.Mod[0:8])
	mod[1] = binary.LittleEndian.Uint64(m.Mod[8:16])
	mod[2] = binary.LittleEndian.Uint64(m.Mod[16:24])
	mod[3] = binary.LittleEndian.Uint64(m.Mod[24:32])
	mod[4] = binary.LittleEndian.Uint64(m.Mod[32:40])
	mod[5] = binary.LittleEndian.Uint64(m.Mod[40:48])

	var c uint64 = 0
	var c1 uint64 = 0
	tmp := [6]uint64{0, 0, 0, 0, 0, 0}

	tmp[0], c = bits.Sub64(x[0], y[0], c)
	tmp[1], c = bits.Sub64(x[1], y[1], c)
	tmp[2], c = bits.Sub64(x[2], y[2], c)
	tmp[3], c = bits.Sub64(x[3], y[3], c)
	tmp[4], c = bits.Sub64(x[4], y[4], c)
	tmp[5], c = bits.Sub64(x[5], y[5], c)

	z[0], c1 = bits.Add64(tmp[0], mod[0], c1)
	z[1], c1 = bits.Add64(tmp[1], mod[1], c1)
	z[2], c1 = bits.Add64(tmp[2], mod[2], c1)
	z[3], c1 = bits.Add64(tmp[3], mod[3], c1)
	z[4], c1 = bits.Add64(tmp[4], mod[4], c1)
	z[5], c1 = bits.Add64(tmp[5], mod[5], c1)

	var src []uint64
	if c == 0 {
		// final sub was unnecessary
		src = tmp[:]
	} else {
		src = z[:]
	}

	binary.LittleEndian.PutUint64(zBytes[0:8], src[0])
	binary.LittleEndian.PutUint64(zBytes[8:16], src[1])
	binary.LittleEndian.PutUint64(zBytes[16:24], src[2])
	binary.LittleEndian.PutUint64(zBytes[24:32], src[3])
	binary.LittleEndian.PutUint64(zBytes[32:40], src[4])
	binary.LittleEndian.PutUint64(zBytes[40:48], src[5])
}
