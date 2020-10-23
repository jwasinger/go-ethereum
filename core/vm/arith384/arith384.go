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

	out[0], c = bits.Add64(x[0], y[0], c)
	out[1], c = bits.Add64(x[1], y[1], c)
	out[2], c = bits.Add64(x[2], y[2], c)
	out[3], c = bits.Add64(x[3], y[3], c)
	out[4], c = bits.Add64(x[4], y[4], c)
	out[5], c = bits.Add64(x[5], y[5], c)
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

		xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[i + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[i + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; i + 0 + k < NUM_LIMBS * 2 && A[i + 0 + k] == ^uint64(0); {
				A[i + 0 + k] = 0
				k++
			}

			if (i + 0 + k < NUM_LIMBS * 2 + 1) {
				A[i + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[i + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[i + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; i + 1 + k < NUM_LIMBS * 2 && A[i + 1 + k] == ^uint64(0); {
				A[i + 1 + k] = 0
				k++
			}

			if (i + 1 + k < NUM_LIMBS * 2 + 1) {
				A[i + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[i + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[i + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; i + 2 + k < NUM_LIMBS * 2 && A[i + 2 + k] == ^uint64(0); {
				A[i + 2 + k] = 0
				k++
			}

			if (i + 2 + k < NUM_LIMBS * 2 + 1) {
				A[i + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[i + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[i + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; i + 3 + k < NUM_LIMBS * 2 && A[i + 3 + k] == ^uint64(0); {
				A[i + 3 + k] = 0
				k++
			}

			if (i + 3 + k < NUM_LIMBS * 2 + 1) {
				A[i + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[i + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[i + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; i + 4 + k < NUM_LIMBS * 2 && A[i + 4 + k] == ^uint64(0); {
				A[i + 4 + k] = 0
				k++
			}

			if (i + 4 + k < NUM_LIMBS * 2 + 1) {
				A[i + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[i], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[i + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[i + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; i + 5 + k < NUM_LIMBS * 2 && A[i + 5 + k] == ^uint64(0); {
				A[i + 5 + k] = 0
				k++
			}

			if (i + 5 + k < NUM_LIMBS * 2 + 1) {
				A[i + 5 + k] += 1
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
