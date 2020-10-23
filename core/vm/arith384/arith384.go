package arith384

import (
	"math/bits"
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
	var xiyj_lo, xiyj_hi uint64 = 0, 0
	var uimj_lo, uimj_hi uint64 = 0, 0
	var partial_sum_lo, partial_sum_hi uint64 = 0, 0
	var sum_lo, sum_hi uint64 = 0, 0

	// var xiyj, uimj, partial_sum, sum uint128.Uint128
	var ui, carry uint64
	var c uint64

	for i := 0; i < NUM_LIMBS; i++ {
		ui = (A[i] + x[i] * y[0]) * inv

		carry = 0
		for j := 0; j < NUM_LIMBS; j++ {

			xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[j])

			uimj_hi, uimj_lo = bits.Mul64(ui, mod[j])

			partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
			partial_sum_hi = xiyj_hi + c

			sum_lo, c = bits.Add64(uimj_lo, A[i + j], 0)
			sum_hi = uimj_hi + c

			sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
			sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

			A[i + j] = sum_lo
			carry = sum_hi

			if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
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
