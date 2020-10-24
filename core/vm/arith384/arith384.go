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


		ui = (A[0] + x[0] * y[0]) * inv

		carry = 0

		xiyj_hi, xiyj_lo = bits.Mul64(x[0], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[0 + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[0 + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 0 + 0 + k < NUM_LIMBS * 2 && A[0 + 0 + k] == ^uint64(0); {
				A[0 + 0 + k] = 0
				k++
			}

			if (0 + 0 + k < NUM_LIMBS * 2 + 1) {
				A[0 + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[0], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[0 + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[0 + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 0 + 1 + k < NUM_LIMBS * 2 && A[0 + 1 + k] == ^uint64(0); {
				A[0 + 1 + k] = 0
				k++
			}

			if (0 + 1 + k < NUM_LIMBS * 2 + 1) {
				A[0 + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[0], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[0 + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[0 + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 0 + 2 + k < NUM_LIMBS * 2 && A[0 + 2 + k] == ^uint64(0); {
				A[0 + 2 + k] = 0
				k++
			}

			if (0 + 2 + k < NUM_LIMBS * 2 + 1) {
				A[0 + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[0], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[0 + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[0 + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 0 + 3 + k < NUM_LIMBS * 2 && A[0 + 3 + k] == ^uint64(0); {
				A[0 + 3 + k] = 0
				k++
			}

			if (0 + 3 + k < NUM_LIMBS * 2 + 1) {
				A[0 + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[0], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[0 + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[0 + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 0 + 4 + k < NUM_LIMBS * 2 && A[0 + 4 + k] == ^uint64(0); {
				A[0 + 4 + k] = 0
				k++
			}

			if (0 + 4 + k < NUM_LIMBS * 2 + 1) {
				A[0 + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[0], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[0 + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[0 + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 0 + 5 + k < NUM_LIMBS * 2 && A[0 + 5 + k] == ^uint64(0); {
				A[0 + 5 + k] = 0
				k++
			}

			if (0 + 5 + k < NUM_LIMBS * 2 + 1) {
				A[0 + 5 + k] += 1
			}
		}

		A[0 + NUM_LIMBS] += carry

		ui = (A[1] + x[1] * y[0]) * inv

		carry = 0

		xiyj_hi, xiyj_lo = bits.Mul64(x[1], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[1 + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[1 + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 1 + 0 + k < NUM_LIMBS * 2 && A[1 + 0 + k] == ^uint64(0); {
				A[1 + 0 + k] = 0
				k++
			}

			if (1 + 0 + k < NUM_LIMBS * 2 + 1) {
				A[1 + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[1], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[1 + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[1 + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 1 + 1 + k < NUM_LIMBS * 2 && A[1 + 1 + k] == ^uint64(0); {
				A[1 + 1 + k] = 0
				k++
			}

			if (1 + 1 + k < NUM_LIMBS * 2 + 1) {
				A[1 + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[1], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[1 + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[1 + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 1 + 2 + k < NUM_LIMBS * 2 && A[1 + 2 + k] == ^uint64(0); {
				A[1 + 2 + k] = 0
				k++
			}

			if (1 + 2 + k < NUM_LIMBS * 2 + 1) {
				A[1 + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[1], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[1 + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[1 + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 1 + 3 + k < NUM_LIMBS * 2 && A[1 + 3 + k] == ^uint64(0); {
				A[1 + 3 + k] = 0
				k++
			}

			if (1 + 3 + k < NUM_LIMBS * 2 + 1) {
				A[1 + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[1], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[1 + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[1 + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 1 + 4 + k < NUM_LIMBS * 2 && A[1 + 4 + k] == ^uint64(0); {
				A[1 + 4 + k] = 0
				k++
			}

			if (1 + 4 + k < NUM_LIMBS * 2 + 1) {
				A[1 + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[1], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[1 + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[1 + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 1 + 5 + k < NUM_LIMBS * 2 && A[1 + 5 + k] == ^uint64(0); {
				A[1 + 5 + k] = 0
				k++
			}

			if (1 + 5 + k < NUM_LIMBS * 2 + 1) {
				A[1 + 5 + k] += 1
			}
		}

		A[1 + NUM_LIMBS] += carry

		ui = (A[2] + x[2] * y[0]) * inv

		carry = 0

		xiyj_hi, xiyj_lo = bits.Mul64(x[2], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[2 + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[2 + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 2 + 0 + k < NUM_LIMBS * 2 && A[2 + 0 + k] == ^uint64(0); {
				A[2 + 0 + k] = 0
				k++
			}

			if (2 + 0 + k < NUM_LIMBS * 2 + 1) {
				A[2 + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[2], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[2 + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[2 + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 2 + 1 + k < NUM_LIMBS * 2 && A[2 + 1 + k] == ^uint64(0); {
				A[2 + 1 + k] = 0
				k++
			}

			if (2 + 1 + k < NUM_LIMBS * 2 + 1) {
				A[2 + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[2], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[2 + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[2 + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 2 + 2 + k < NUM_LIMBS * 2 && A[2 + 2 + k] == ^uint64(0); {
				A[2 + 2 + k] = 0
				k++
			}

			if (2 + 2 + k < NUM_LIMBS * 2 + 1) {
				A[2 + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[2], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[2 + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[2 + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 2 + 3 + k < NUM_LIMBS * 2 && A[2 + 3 + k] == ^uint64(0); {
				A[2 + 3 + k] = 0
				k++
			}

			if (2 + 3 + k < NUM_LIMBS * 2 + 1) {
				A[2 + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[2], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[2 + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[2 + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 2 + 4 + k < NUM_LIMBS * 2 && A[2 + 4 + k] == ^uint64(0); {
				A[2 + 4 + k] = 0
				k++
			}

			if (2 + 4 + k < NUM_LIMBS * 2 + 1) {
				A[2 + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[2], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[2 + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[2 + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 2 + 5 + k < NUM_LIMBS * 2 && A[2 + 5 + k] == ^uint64(0); {
				A[2 + 5 + k] = 0
				k++
			}

			if (2 + 5 + k < NUM_LIMBS * 2 + 1) {
				A[2 + 5 + k] += 1
			}
		}

		A[2 + NUM_LIMBS] += carry

		ui = (A[3] + x[3] * y[0]) * inv

		carry = 0

		xiyj_hi, xiyj_lo = bits.Mul64(x[3], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[3 + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[3 + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 3 + 0 + k < NUM_LIMBS * 2 && A[3 + 0 + k] == ^uint64(0); {
				A[3 + 0 + k] = 0
				k++
			}

			if (3 + 0 + k < NUM_LIMBS * 2 + 1) {
				A[3 + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[3], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[3 + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[3 + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 3 + 1 + k < NUM_LIMBS * 2 && A[3 + 1 + k] == ^uint64(0); {
				A[3 + 1 + k] = 0
				k++
			}

			if (3 + 1 + k < NUM_LIMBS * 2 + 1) {
				A[3 + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[3], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[3 + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[3 + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 3 + 2 + k < NUM_LIMBS * 2 && A[3 + 2 + k] == ^uint64(0); {
				A[3 + 2 + k] = 0
				k++
			}

			if (3 + 2 + k < NUM_LIMBS * 2 + 1) {
				A[3 + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[3], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[3 + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[3 + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 3 + 3 + k < NUM_LIMBS * 2 && A[3 + 3 + k] == ^uint64(0); {
				A[3 + 3 + k] = 0
				k++
			}

			if (3 + 3 + k < NUM_LIMBS * 2 + 1) {
				A[3 + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[3], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[3 + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[3 + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 3 + 4 + k < NUM_LIMBS * 2 && A[3 + 4 + k] == ^uint64(0); {
				A[3 + 4 + k] = 0
				k++
			}

			if (3 + 4 + k < NUM_LIMBS * 2 + 1) {
				A[3 + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[3], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[3 + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[3 + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 3 + 5 + k < NUM_LIMBS * 2 && A[3 + 5 + k] == ^uint64(0); {
				A[3 + 5 + k] = 0
				k++
			}

			if (3 + 5 + k < NUM_LIMBS * 2 + 1) {
				A[3 + 5 + k] += 1
			}
		}

		A[3 + NUM_LIMBS] += carry

		ui = (A[4] + x[4] * y[0]) * inv

		carry = 0

		xiyj_hi, xiyj_lo = bits.Mul64(x[4], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[4 + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[4 + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 4 + 0 + k < NUM_LIMBS * 2 && A[4 + 0 + k] == ^uint64(0); {
				A[4 + 0 + k] = 0
				k++
			}

			if (4 + 0 + k < NUM_LIMBS * 2 + 1) {
				A[4 + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[4], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[4 + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[4 + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 4 + 1 + k < NUM_LIMBS * 2 && A[4 + 1 + k] == ^uint64(0); {
				A[4 + 1 + k] = 0
				k++
			}

			if (4 + 1 + k < NUM_LIMBS * 2 + 1) {
				A[4 + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[4], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[4 + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[4 + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 4 + 2 + k < NUM_LIMBS * 2 && A[4 + 2 + k] == ^uint64(0); {
				A[4 + 2 + k] = 0
				k++
			}

			if (4 + 2 + k < NUM_LIMBS * 2 + 1) {
				A[4 + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[4], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[4 + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[4 + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 4 + 3 + k < NUM_LIMBS * 2 && A[4 + 3 + k] == ^uint64(0); {
				A[4 + 3 + k] = 0
				k++
			}

			if (4 + 3 + k < NUM_LIMBS * 2 + 1) {
				A[4 + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[4], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[4 + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[4 + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 4 + 4 + k < NUM_LIMBS * 2 && A[4 + 4 + k] == ^uint64(0); {
				A[4 + 4 + k] = 0
				k++
			}

			if (4 + 4 + k < NUM_LIMBS * 2 + 1) {
				A[4 + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[4], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[4 + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[4 + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 4 + 5 + k < NUM_LIMBS * 2 && A[4 + 5 + k] == ^uint64(0); {
				A[4 + 5 + k] = 0
				k++
			}

			if (4 + 5 + k < NUM_LIMBS * 2 + 1) {
				A[4 + 5 + k] += 1
			}
		}

		A[4 + NUM_LIMBS] += carry

		ui = (A[5] + x[5] * y[0]) * inv

		carry = 0

		xiyj_hi, xiyj_lo = bits.Mul64(x[5], y[0])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[0])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[5 + 0], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[5 + 0] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 5 + 0 + k < NUM_LIMBS * 2 && A[5 + 0 + k] == ^uint64(0); {
				A[5 + 0 + k] = 0
				k++
			}

			if (5 + 0 + k < NUM_LIMBS * 2 + 1) {
				A[5 + 0 + k] += 1
			}
		}
		
		// 2 
		xiyj_hi, xiyj_lo = bits.Mul64(x[5], y[1])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[1])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[5 + 1], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[5 + 1] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 5 + 1 + k < NUM_LIMBS * 2 && A[5 + 1 + k] == ^uint64(0); {
				A[5 + 1 + k] = 0
				k++
			}

			if (5 + 1 + k < NUM_LIMBS * 2 + 1) {
				A[5 + 1 + k] += 1
			}
		}
		
		// 3

		xiyj_hi, xiyj_lo = bits.Mul64(x[5], y[2])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[2])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[5 + 2], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[5 + 2] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 5 + 2 + k < NUM_LIMBS * 2 && A[5 + 2 + k] == ^uint64(0); {
				A[5 + 2 + k] = 0
				k++
			}

			if (5 + 2 + k < NUM_LIMBS * 2 + 1) {
				A[5 + 2 + k] += 1
			}
		}

		// 4

		xiyj_hi, xiyj_lo = bits.Mul64(x[5], y[3])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[3])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[5 + 3], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[5 + 3] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 5 + 3 + k < NUM_LIMBS * 2 && A[5 + 3 + k] == ^uint64(0); {
				A[5 + 3 + k] = 0
				k++
			}

			if (5 + 3 + k < NUM_LIMBS * 2 + 1) {
				A[5 + 3 + k] += 1
			}
		}

		// 5

		xiyj_hi, xiyj_lo = bits.Mul64(x[5], y[4])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[4])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[5 + 4], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[5 + 4] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 5 + 4 + k < NUM_LIMBS * 2 && A[5 + 4 + k] == ^uint64(0); {
				A[5 + 4 + k] = 0
				k++
			}

			if (5 + 4 + k < NUM_LIMBS * 2 + 1) {
				A[5 + 4 + k] += 1
			}
		}

		// 6

		xiyj_hi, xiyj_lo = bits.Mul64(x[5], y[5])

		uimj_hi, uimj_lo = bits.Mul64(ui, mod[5])

		partial_sum_lo, c = bits.Add64(xiyj_lo, carry, 0)
		partial_sum_hi = xiyj_hi + c

		sum_lo, c = bits.Add64(uimj_lo, A[5 + 5], 0)
		sum_hi = uimj_hi + c

		sum_lo, c = bits.Add64(partial_sum_lo, sum_lo, 0)
		sum_hi, _ = bits.Add64(partial_sum_hi, sum_hi, c)

		A[5 + 5] = sum_lo
		carry = sum_hi

		if lt(sum_hi, sum_lo, partial_sum_hi, partial_sum_lo) {
			var k int
			k = 2
			for ; 5 + 5 + k < NUM_LIMBS * 2 && A[5 + 5 + k] == ^uint64(0); {
				A[5 + 5 + k] = 0
				k++
			}

			if (5 + 5 + k < NUM_LIMBS * 2 + 1) {
				A[5 + 5 + k] += 1
			}
		}

		A[5 + NUM_LIMBS] += carry

	for i := 0; i < NUM_LIMBS; i++ {
		out[i] = A[i + NUM_LIMBS]
	}

	if lte(mod, out) {
		Sub(out, out, mod)
	}
}
