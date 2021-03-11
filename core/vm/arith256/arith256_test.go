package arith256

import (
	"testing"
)

func BenchmarkAddMod_4limbs(b *testing.B) {
	var x, y, r_squared, mod Element
	var modinv uint64 = 14042775128853446655 // pow(-mod, -1, 1<<64)

	mod = *ElementFromString("21888242871839275222246405745257275088548364400416034343698204186575808495617")
	r_squared = *ElementFromString("944936681149208446651664254269745548490766851729442924617792859073125903783")

	x = *ElementFromString("3")
	y = *ElementFromString("2")

	x.ToMont(&mod, &r_squared, modinv)
	y.ToMont(&mod, &r_squared, modinv)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		x.AddMod(&x, &y, &mod)
	}
}

func BenchmarkSubMod256(b *testing.B) {
	var x, y, r_squared, mod Element
	var modinv uint64 = 14042775128853446655

	mod = *ElementFromString("21888242871839275222246405745257275088548364400416034343698204186575808495617")
	r_squared = *ElementFromString("944936681149208446651664254269745548490766851729442924617792859073125903783")

	x = *ElementFromString("3")
	y = *ElementFromString("2")

	x.ToMont(&mod, &r_squared, modinv)
	y.ToMont(&mod, &r_squared, modinv)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		x.SubMod(&x, &y, &mod)
	}
}

func BenchmarkMulMod256(b *testing.B) {
	var x, y, r_squared, mod Element
	var modinv uint64 = 14042775128853446655

	mod = *ElementFromString("21888242871839275222246405745257275088548364400416034343698204186575808495617")
	r_squared = *ElementFromString("944936681149208446651664254269745548490766851729442924617792859073125903783")

	x = *ElementFromString("2")
	y = *ElementFromString("3")

	x.ToMont(&mod, &r_squared, modinv)
	y.ToMont(&mod, &r_squared, modinv)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		x.MulModMont(&x, &y, &mod, modinv)
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

	// modulus = order of group on BN128 curve , r = 1<<256 % modulus, r_squared = r**2 % modulus
	mod = *ElementFromString("21888242871839275222246405745257275088548364400416034343698204186575808495617")
	r_squared = *ElementFromString("944936681149208446651664254269745548490766851729442924617792859073125903783")
	modinv = 14042775128853446655 // pow(-mod, -1, 1<<64)

	x = *ElementFromString("2")
	y = *ElementFromString("3")
	expected = *ElementFromString("6")

	x.ToMont(&mod, &r_squared, modinv)
	y.ToMont(&mod, &r_squared, modinv)

	x.MulModMont(&x, &y, &mod, modinv)
	x.ToNorm(&mod, modinv)

	if !x.Eq(&expected) {
		t.Fatalf("neq")
	}
}
