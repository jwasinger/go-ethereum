package evmmax

import (
	"crypto/rand"
	"math/big"
	"testing"
)

type opType = uint

const (
	op_addmod opType = iota
	op_submod
	op_mulmont
)

func randInt(max *big.Int) *big.Int {
	// Generate cryptographically strong pseudo-random between [0, max)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err)
	}
	return n
}

func TestSubModBLS12381Random(t *testing.T) {
	for i := 0; i < 10000; i++ {
		bls12381Modulus, _ := new(big.Int).SetString("1a0111ea397fe69a4b1ba7b6434bacd764774b84f38512bf6730d2a0f6b0f6241eabfffeb153ffffb9feffffffffaaab", 16)
		x := randInt(bls12381Modulus)
		y := randInt(bls12381Modulus)
		expected := new(big.Int)
		expected = expected.Sub(x, y)
		expected = expected.Mod(expected, bls12381Modulus)

		testOp(t, op_submod, x, y, bls12381Modulus, expected, 0, 1, 2)

		expectedZero := big.NewInt(0)
		// test overlapping inputs
		testOp(t, op_submod, x, x, bls12381Modulus, expectedZero, 0, 1, 1)
		// test overlapping inputs/outputs
		testOp(t, op_submod, x, x, bls12381Modulus, expectedZero, 1, 1, 1)
	}
}

func TestAddModBLS12381Random(t *testing.T) {
	for i := 0; i < 10000; i++ {
		bls12381Modulus, _ := new(big.Int).SetString("1a0111ea397fe69a4b1ba7b6434bacd764774b84f38512bf6730d2a0f6b0f6241eabfffeb153ffffb9feffffffffaaab", 16)
		x := randInt(bls12381Modulus)
		y := randInt(bls12381Modulus)
		expected := new(big.Int)
		expected = expected.Add(x, y)
		expected = expected.Mod(expected, bls12381Modulus)

		testOp(t, op_addmod, x, y, bls12381Modulus, expected, 0, 1, 2)

		expectedDbl := new(big.Int).Add(x, x)
		expectedDbl = expectedDbl.Mod(expectedDbl, bls12381Modulus)
		// test squaring with overlapping inputs
		testOp(t, op_addmod, x, x, bls12381Modulus, expectedDbl, 0, 1, 1)
		// test squaring with overlapping inputs/outputs
		testOp(t, op_addmod, x, x, bls12381Modulus, expectedDbl, 1, 1, 1)
	}
}

func TestMulModBLS12381Random(t *testing.T) {
	for i := 0; i < 10000; i++ {
		bls12381Modulus, _ := new(big.Int).SetString("1a0111ea397fe69a4b1ba7b6434bacd764774b84f38512bf6730d2a0f6b0f6241eabfffeb153ffffb9feffffffffaaab", 16)
		x := randInt(bls12381Modulus)
		y := randInt(bls12381Modulus)
		expected := new(big.Int)
		expected = expected.Mul(x, y)
		expected = expected.Mod(expected, bls12381Modulus)

		testOp(t, op_mulmont, x, y, bls12381Modulus, expected, 0, 1, 2)

		expectedSqr := new(big.Int).Mul(x, x)
		expectedSqr = expectedSqr.Mod(expectedSqr, bls12381Modulus)
		// test squaring with overlapping inputs
		testOp(t, op_mulmont, x, x, bls12381Modulus, expectedSqr, 0, 1, 1)
		// test squaring with overlapping inputs/outputs
		testOp(t, op_mulmont, x, x, bls12381Modulus, expectedSqr, 1, 1, 1)
	}
}

func BenchmarkMulModBLS12381Random(b *testing.B) {
	bls12381Modulus, _ := new(big.Int).SetString("1a0111ea397fe69a4b1ba7b6434bacd764774b84f38512bf6730d2a0f6b0f6241eabfffeb153ffffb9feffffffffaaab", 16)
	x := randInt(bls12381Modulus)
	y := randInt(bls12381Modulus)
	expected := new(big.Int)
	expected = expected.Mul(x, y)
	expected = expected.Mod(expected, bls12381Modulus)

	benchOp(b, op_mulmont, x, y, bls12381Modulus, expected, 0, 1, 2)
}

func benchOp(b *testing.B, op opType, x, y, mod, expected *big.Int, outOffset, xOffset, yOffset int) {
	modBytes := mod.Bytes()
	if len(modBytes)+len(modBytes)%8 != 48 {
		panic("modBytes must be representable in 321-384 bits")
	}

	e := EVMMAXState{nil, make(map[uint]ModState)}
	if err := e.Setup(0, 3, modBytes); err != nil {
		panic("setup failed")
	}

	elemSize := int(e.ActiveModulus.ElemSize)

	if x.Cmp(mod) >= 0 || y.Cmp(mod) >= 0 {
		panic("x/y must be less than the modulus")
	}

	valsBytes := make([]byte, elemSize*3)
	xBytes := x.Bytes()
	yBytes := y.Bytes()
	copy(valsBytes[elemSize+(48-len(xBytes)):elemSize*2], xBytes[:])
	copy(valsBytes[elemSize*2+(48-len(yBytes)):], yBytes[:])
	if err := e.ActiveModulus.StoreValues(valsBytes, 0); err != nil {
		panic("store failed")
	}

	outBytes := e.ActiveModulus.Memory[outOffset*elemSize : (outOffset+1)*elemSize]
	xBytes = e.ActiveModulus.Memory[xOffset*elemSize : (xOffset+1)*elemSize]
	yBytes = e.ActiveModulus.Memory[yOffset*elemSize : (yOffset+1)*elemSize]

	var benchFn arithFunc

	switch op {
	case op_addmod:
		benchFn = e.ActiveModulus.AddMod
	case op_submod:
		benchFn = e.ActiveModulus.SubMod
	case op_mulmont:
		benchFn = e.ActiveModulus.MulMont
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchFn(e.ActiveModulus, outBytes, xBytes, yBytes)
	}
}

func testOp(t *testing.T, op opType, x, y, mod, expected *big.Int, outOffset, xOffset, yOffset int) {
	modBytes := mod.Bytes()
	if len(modBytes)+len(modBytes)%8 != 48 {
		t.Fatalf("modBytes must be representable in 321-384 bits")
	}

	e := EVMMAXState{nil, make(map[uint]ModState)}
	if err := e.Setup(0, 3, modBytes); err != nil {
		t.Fatalf("setup failed")
	}

	elemSize := int(e.ActiveModulus.ElemSize)

	if x.Cmp(mod) >= 0 || y.Cmp(mod) >= 0 {
		t.Fatalf("x/y must be less than the modulus")
	}

	valsBytes := make([]byte, elemSize*3)
	xBytes := x.Bytes()
	yBytes := y.Bytes()
	copy(valsBytes[elemSize+(48-len(xBytes)):elemSize*2], xBytes[:])
	copy(valsBytes[elemSize*2+(48-len(yBytes)):], yBytes[:])
	if err := e.ActiveModulus.StoreValues(valsBytes, 0); err != nil {
		panic("store failed")
	}

	outBytes := e.ActiveModulus.Memory[outOffset*elemSize : (outOffset+1)*elemSize]
	xBytes = e.ActiveModulus.Memory[xOffset*elemSize : (xOffset+1)*elemSize]
	yBytes = e.ActiveModulus.Memory[yOffset*elemSize : (yOffset+1)*elemSize]

	switch op {
	case op_addmod:
		e.ActiveModulus.AddMod(e.ActiveModulus, outBytes, xBytes, yBytes)
	case op_submod:
		e.ActiveModulus.SubMod(e.ActiveModulus, outBytes, xBytes, yBytes)
	case op_mulmont:
		e.ActiveModulus.MulMont(e.ActiveModulus, outBytes, xBytes, yBytes)
	}
	e.ActiveModulus.LoadValues(valsBytes, 0)

	outBytes = valsBytes[outOffset*elemSize : (outOffset+1)*elemSize]
	xBytes = valsBytes[xOffset*elemSize : (xOffset+1)*elemSize]
	yBytes = valsBytes[yOffset*elemSize : (yOffset+1)*elemSize]

	if outOffset == xOffset {
		if outOffset == yOffset {
			correct := new(big.Int).SetBytes(xBytes).Cmp(new(big.Int).SetBytes(outBytes)) == 0 &&
				new(big.Int).SetBytes(yBytes).Cmp(new(big.Int).SetBytes(outBytes)) == 0 &&
				new(big.Int).SetBytes(outBytes).Cmp(expected) == 0
			if !correct {
				t.Fatalf("bad")
			}
		} else {
			correct := new(big.Int).SetBytes(xBytes).Cmp(new(big.Int).SetBytes(outBytes)) == 0 &&
				new(big.Int).SetBytes(yBytes).Cmp(y) == 0 &&
				new(big.Int).SetBytes(outBytes).Cmp(expected) == 0
			if !correct {
				t.Fatalf("bad")
			}
		}
	} else if outOffset == yOffset {
		if outOffset == xOffset {
			correct := new(big.Int).SetBytes(xBytes).Cmp(new(big.Int).SetBytes(outBytes)) == 0 &&
				new(big.Int).SetBytes(yBytes).Cmp(new(big.Int).SetBytes(outBytes)) == 0 &&
				new(big.Int).SetBytes(outBytes).Cmp(expected) == 0
			if !correct {
				t.Fatalf("bad")
			}
		} else {
			correct := new(big.Int).SetBytes(yBytes).Cmp(new(big.Int).SetBytes(outBytes)) == 0 &&
				new(big.Int).SetBytes(xBytes).Cmp(x) == 0 &&
				new(big.Int).SetBytes(outBytes).Cmp(expected) == 0
			if !correct {
				t.Fatalf("bad")
			}
		}
	} else {
		correct := new(big.Int).SetBytes(yBytes).Cmp(y) == 0 &&
			new(big.Int).SetBytes(xBytes).Cmp(x) == 0 &&
			new(big.Int).SetBytes(outBytes).Cmp(expected) == 0
		if !correct {
			t.Fatalf("bad")
		}
	}
}
