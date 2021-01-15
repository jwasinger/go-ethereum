package arith384

import (
	"math/bits"
    "math/big"
    "fmt"
)

const NUM_LIMBS = 6
type Element [NUM_LIMBS]uint64

// out <- x + y
func add(out *Element, x *Element, y *Element) {
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
func sub(out *Element, x *Element, y *Element) (uint64){
	var c uint64
	c = 0

	out[0], c = bits.Sub64(x[0], y[0], c)
	out[1], c = bits.Sub64(x[1], y[1], c)
	out[2], c = bits.Sub64(x[2], y[2], c)
	out[3], c = bits.Sub64(x[3], y[3], c)
	out[4], c = bits.Sub64(x[4], y[4], c)
	out[5], c = bits.Sub64(x[5], y[5], c)

	return c
}

func Eq(x *Element, y *Element) bool {
    for i := 0; i < NUM_LIMBS; i++ {
        if x[i] != y[i] {
            return false
        }
    }

    return true
}

/*
	Modular Addition
*/
func AddMod(out *Element, x *Element, y *Element, mod *Element) {
	var c uint64
	var tmp Element

	tmp[0], c = bits.Add64(x[0], y[0], 0)
	tmp[1], c = bits.Add64(x[1], y[1], c)
	tmp[2], c = bits.Add64(x[2], y[2], c)
	tmp[3], c = bits.Add64(x[3], y[3], c)
	tmp[4], c = bits.Add64(x[4], y[4], c)
	tmp[5], c = bits.Add64(x[5], y[5], c)

	out[0], c = bits.Sub64(tmp[0], mod[0], 0)
	out[1], c = bits.Sub64(tmp[1], mod[1], c)
	out[2], c = bits.Sub64(tmp[2], mod[2], c)
	out[3], c = bits.Sub64(tmp[3], mod[3], c)
	out[4], c = bits.Sub64(tmp[4], mod[4], c)
	out[5], c = bits.Sub64(tmp[5], mod[5], c)

	// TODO shouldn't check for carry here use carry from before Sub64s ?
	if c != 0 { // unnecessary sub
		*out = tmp
	}
}

/*
	Modular Subtraction
*/
func SubMod(out *Element, x *Element, y *Element, mod *Element) {
	var c, c1 uint64
	var tmp Element

	tmp[0], c1 = bits.Sub64(x[0], y[0], 0)
	tmp[1], c1 = bits.Sub64(x[1], y[1], c1)
	tmp[2], c1 = bits.Sub64(x[2], y[2], c1)
	tmp[3], c1 = bits.Sub64(x[3], y[3], c1)
	tmp[4], c1 = bits.Sub64(x[4], y[4], c1)
	tmp[5], c1 = bits.Sub64(x[5], y[5], c1)

	out[0], c = bits.Add64(tmp[0], mod[0], 0)
	out[1], c = bits.Add64(tmp[1], mod[1], c)
	out[2], c = bits.Add64(tmp[2], mod[2], c)
	out[3], c = bits.Add64(tmp[3], mod[3], c)
	out[4], c = bits.Add64(tmp[4], mod[4], c)
	out[5], c = bits.Add64(tmp[5], mod[5], c)

	if c1 == 0 { // unnecessary add
		*out = tmp
	}
}

func lte(x *Element, y *Element) bool {
	for i := 0; i < NUM_LIMBS; i++ {
		if x[i] > y[i] {
			return false
		}
	}

	return true
}

// returns True if x < y
func lt384(x *Element, y *Element) bool {
	_, carry := bits.Sub64(x[0], y[0], 0)
	_, carry = bits.Sub64(x[1], y[1], carry)
	_, carry = bits.Sub64(x[2], y[2], carry)
	_, carry = bits.Sub64(x[3], y[3], carry)
	_, carry = bits.Sub64(x[4], y[4], carry)
	_, carry = bits.Sub64(x[5], y[5], carry)
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

func MulModNaive(out *big.Int, x *big.Int, y *big.Int) {
    // hardcode to bn128 Fr parameters
	r_inv := new(big.Int)
	mod := new(big.Int)

	mod.SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)
	r_inv.SetString("9915499612839321149637521777990102151350674507940716049588462388200839649614", 10)

	out.Mul(x, y)
	out.Mul(out, r_inv)
	out.Mod(out, mod)
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

    fmt.Printf("%x\n", A)
    for i := 0; i < NUM_LIMBS; i++ {
        out[i] = A[i + NUM_LIMBS]
    }

    if lte(mod, out) {
        sub(out, out, mod)
    }
}
