package arith384

import (
	"math/bits"
	"github.com/jwasinger/uint128"
)

const NUM_LIMBS = 6
type Element [NUM_LIMBS]uint64

// out <- x + y
func Add(out *Element, x *Element, y *Element) {
	var c uint64
	c = 0

	// TODO: manually unroll this?
	for i := 0; i < NUM_LIMBS; i++ {
		// out[i] = x[i] + y[i] + c
		out[i], c = bits.Add64(x[i], y[i], c)
	}
}

// out <- x - y
func Sub(out *Element, x *Element, y *Element) (uint64){
	var c uint64
	c = 0

	for i := 0; i < NUM_LIMBS; i++ {
		out[i], c = bits.Sub64(x[i], y[i], c)
	}

	return c
}

// return x <= y
func lte(x *Element, y *Element) bool {
	for i := 0; i < NUM_LIMBS; i++ {
		if x[i] > y[i] {
			return false
		}
	}

	return true
}

/*
	Modular Addition
*/
func AddMod(out *Element, x *Element, y *Element, mod *Element) {
	Add(out, x, y)

	if lte(mod, out) {
		Sub(out, out, mod)
	}
}

/*
	Modular Subtraction
*/
func SubMod(out *Element, x *Element, y *Element, mod *Element) {
	var c uint64
	c = Sub(out, x, y)

	// if result < 0 -> result += mod
	if c != 0 {
		Add(out, out, mod)
	}
}

// return True if a < b, else False
func lt(a_hi, a_lo, b_hi, b_lo uint64) bool {
	if a_hi < b_hi || (a_hi == b_hi && a_lo < b_lo) {
		return true
	} else {
		return false
	}
}

/*
	Montgomery Modular Multiplication: algorithm 14.36, Handbook of Applied Cryptography, http://cacr.uwaterloo.ca/hac/about/chap14.pdf
*/
func MulMod(out *Element, x *Element, y *Element, mod *Element, inv uint64) {
	var A [NUM_LIMBS * 2 + 1]uint64
	var xiyj, uimj, partial_sum, sum uint128.Uint128
	var ui, carry uint64
	var c uint64

	xiyj = uint128.New(0, 0)
	uimj = uint128.New(0, 0)

	for i := 0; i < NUM_LIMBS; i++ {
		ui = (A[i] + x[i] * y[0]) * inv

		carry = 0
		for j := 0; j < NUM_LIMBS; j++ {

			xiyj.Hi, xiyj.Lo = bits.Mul64(x[i], y[j])

			uimj.Hi, uimj.Lo = bits.Mul64(ui, mod[j])

			partial_sum.Lo, c = bits.Add64(xiyj.Lo, carry, 0)
			partial_sum.Hi = xiyj.Hi + c

			sum.Lo, c = bits.Add64(uimj.Lo, A[i + j], 0)
			sum.Hi = uimj.Hi + c

			sum.Lo, c = bits.Add64(partial_sum.Lo, sum.Lo, 0)
			sum.Hi, _ = bits.Add64(partial_sum.Hi, sum.Hi, c)

			A[i + j] = sum.Lo
			carry = sum.Hi

			if lt(sum.Hi, sum.Lo, partial_sum.Hi, partial_sum.Lo) {
				var k int
				k = 2
				for ; i + j + k < NUM_LIMBS * 2 && A[i + j + k] == ^uint64(0); {
					A[i + j + k] = 0
					k++
				}

				if (i + j + k < NUM_LIMBS * 2 + 1) {
					A[i + j + k] += 1
				}
			}
		}

		A[i + NUM_LIMBS] += carry
	}

	for i := 0; i < NUM_LIMBS; i++ {
		out[i] = A[i + NUM_LIMBS]
	}

	if lte(mod, out) {
		Sub(out, out, mod)
	}
}
