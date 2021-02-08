package arith384

import (
	"math/bits"
)

type Element []uint64

// out <- x + y
func add(out *Element, x *Element, y *Element, num_limbs int) (uint64) {
	var c uint64
	c = 0

    for i := 0; i < num_limbs; i++ {
        (*out)[i], c = bits.Add64((*x)[i], (*y)[i], c)
    }

    return c
}

// out <- x - y
func sub(out *Element, x *Element, y *Element, num_limbs int) (uint64){
	var c uint64
	c = 0

    for i := 0; i < num_limbs; i++ {
        (*out)[i], c = bits.Add64((*x)[i], (*y)[i], c)
    }

	return c
}

func Eq(x *Element, y *Element, num_limbs int) bool {
    for i := 0; i < num_limbs; i++ {
        if (*x)[i] != (*y)[i] {
            return false
        }
    }

    return true
}

/*
	Modular Addition
*/
func AddMod(out *Element, x *Element, y *Element, mod *Element, num_limbs int) {
	var c uint64
	var tmp Element

    add(&tmp, x, y, num_limbs)
    c = sub(out, &tmp, mod, num_limbs)

	// TODO shouldn't check for carry here use carry from before Sub64s ?
	if c != 0 { // unnecessary sub
		*out = tmp
	}
}

/*
	Modular Subtraction
*/
func SubMod(out *Element, x *Element, y *Element, mod *Element, num_limbs int) {
	var c uint64
	var tmp Element

    sub(&tmp, x, y, num_limbs)
    c = add(out, &tmp, mod, num_limbs)

	// TODO shouldn't check for carry here use carry from before Sub64s ?
	if c != 0 { // unnecessary sub
		*out = tmp
	}
}

func lte(x *Element, y *Element, num_limbs int) bool {
	for i := 0; i < num_limbs; i++ {
		if (*x)[i] > (*y)[i] {
			return false
		}
	}

	return true
}

// returns True if x < y
func lt384(x *Element, y *Element, num_limbs int) bool {
    var carry uint64 = 0

    for i := 0; i < num_limbs; i++ {
	    _, carry = bits.Sub64((*x)[i], (*y)[i], carry)
    }

	return carry != 0
}

// return True if a < b, else False
func lt(a_hi, a_lo, b_hi, b_lo uint64) bool {
	return a_hi < b_hi || (a_hi == b_hi && a_lo < b_lo)

	/*
	// the below should be faster... but it slows down the MulModMont benchmark by ~15%
	_, carry := bits.Sub64(a_lo, b_lo, 0)
	_, carry = bits.Sub64(a_hi, b_hi, carry)

	return carry != 0
	*/
}


/*
    Montgomery Modular Multiplication: algorithm 14.36, Handbook of Applied Cryptography, http://cacr.uwaterloo.ca/hac/about/chap14.pdf
*/
func MulMod(out *Element, x *Element, y *Element, mod *Element, inv uint64, num_limbs int) {
    A := make([]uint64, num_limbs * 2 + 1, num_limbs * 2 + 1)
    var xiyj_lo, xiyj_hi uint64 = 0, 0
    var uimj_lo, uimj_hi uint64 = 0, 0
    var partial_sum_lo, partial_sum_hi uint64 = 0, 0
    var sum_lo, sum_hi uint64 = 0, 0

    // var xiyj, uimj, partial_sum, sum uint128.Uint128
    var ui, carry uint64
    var c uint64

    for i := 0; i < num_limbs; i++ {
        ui = (A[i] + (*x)[i] * (*y)[0]) * inv

        carry = 0
        for j := 0; j < num_limbs; j++ {

            xiyj_hi, xiyj_lo = bits.Mul64((*x)[i], (*y)[j])

            uimj_hi, uimj_lo = bits.Mul64(ui, (*mod)[j])

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
                for ; i + j + k < num_limbs * 2 && A[i + j + k] == ^uint64(0); {
                    A[i + j + k] = 0
                    k++
                }

                if (i + j + k < num_limbs * 2 + 1) {
                    A[i + j + k] += 1
                }
            }
        }

        A[i + num_limbs] += carry
    }

    for i := 0; i < num_limbs; i++ {
        (*out)[i] = A[i + num_limbs]
    }

    if lte(mod, out, num_limbs) {
        sub(out, out, mod, num_limbs)
    }
}
