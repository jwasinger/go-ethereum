package arith384

import (
"testing"
)

func BenchmarkAddMod_4limbs(b *testing.B) {
	mod := Element{0xb9feffffffffaaab, 0x1eabfffeb153ffff, 0x6730d2a0f6b0f624, 0x64774b84f38512bf}

	x := Element{0x20b39e434f6b7627, 0xe3b9585c3bc798c3, 0xd601841435360731, 0x592efb881d54c66d}
	y := Element{0xd2f66b13d3e3cc9e, 0xc4ad7d09d3b8497d, 0xfc3bcaaeef9fd81e, 0x55ff24e182d1d704}

	for n := 0; n < b.N; n++ {
		AddMod(&x, &x, &y, &mod)
	}
}


func BenchmarkSubMod_4limbs(b *testing.B) {
        mod := Element{0xb9feffffffffaaab, 0x1eabfffeb153ffff, 0x6730d2a0f6b0f624, 0x64774b84f38512bf}

        x := Element{0x20b39e434f6b7627, 0xe3b9585c3bc798c3, 0xd601841435360731, 0x592efb881d54c66d}
        y := Element{0xd2f66b13d3e3cc9e, 0xc4ad7d09d3b8497d, 0xfc3bcaaeef9fd81e, 0x55ff24e182d1d704}

        for n := 0; n < b.N; n++ {
                SubMod(&x, &x, &y, &mod)
        }
}


func BenchmarkMulMod_4limbs(b *testing.B) {
	x := Element{0xb1f598e5f390298f, 0x6b3088c3a380f4b8, 0x4d10c051c1fa23c0, 0x2945981a13aec13}
	y := Element{0x4c64af08c847d3ec, 0xf47665551a973a7a, 0x4f0090b4b602e334, 0x670a33daa7a418b4}
	mod := Element{0xb9feffffffffaaab, 0x1eabfffeb153ffff, 0x6730d2a0f6b0f624, 0x64774b84f38512bf}
	var inv uint64
	inv = 0x89f3fffcfffcfffd

	for n := 0; n < b.N; n++ {
		MulMod(&x, &x, &y, &mod, inv)
	}
}

func TestStringConversion(t *testing.T) {
	x := *ElementFromString("21888242871839275222246405745257275088696311157297823662689037894645226208583")

	if x.String() != "21888242871839275222246405745257275088696311157297823662689037894645226208583" {
		//t.Fatalf("%s %x bad", x.String(), x)
		t.Fatal()
	}
}

func TestMulModMont(t *testing.T) {
	var x, y, r_squared, expected, mod Element
	var modinv uint64

	// modulus = BN128 curve order, r = 1<<256 % modulus, r_squared = r**2 % modulus
	mod = *ElementFromString("21888242871839275222246405745257275088696311157297823662689037894645226208583")
	r_squared = *ElementFromString("3096616502983703923843567936837374451735540968419076528771170197431451843209")
	modinv = 9786893198990664585

	x = *ElementFromString("3")
	y = *ElementFromString("2")
	expected = *ElementFromString("6")

	x.ToMont(&mod, &r_squared, modinv)
	y.ToMont(&mod, &r_squared, modinv)

	x.MulModMont(&x, &y, &mod, modinv)
	x.ToNorm(&mod, modinv)

	if !x.Eq(&expected) {
		t.Fatalf("neq")
	}
}
