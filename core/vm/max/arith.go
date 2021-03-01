package max

import (
    "unsafe"
)

func MulMod(limb_count int64, out []byte, x []byte, y []byte, mod []byte, modinv uint64) {
    switch limb_count {
        case 4:
            MulMod256(
                ((*Element256) (unsafe.Pointer(&out))),
                ((*Element256) (unsafe.Pointer(&x))),
                ((*Element256) (unsafe.Pointer(&y))),
                ((*Element256) (unsafe.Pointer(&mod))),
                modinv)
        case 6:
            MulMod384(
                ((*Element384) (unsafe.Pointer(&out))),
                ((*Element384) (unsafe.Pointer(&x))),
                ((*Element384) (unsafe.Pointer(&y))),
                ((*Element384) (unsafe.Pointer(&mod))),
                modinv)
        default:
            panic("this case should have been checked before hitting here :)")
    }
}

func AddMod(limb_count int64, out []byte, x []byte, y []byte, mod []byte) {
    switch limb_count {
        case 4:
            AddMod256(
                ((*Element256) (unsafe.Pointer(&out))),
                ((*Element256) (unsafe.Pointer(&x))),
                ((*Element256) (unsafe.Pointer(&y))),
                ((*Element256) (unsafe.Pointer(&mod))))
        case 6:
            AddMod384(
                ((*Element384) (unsafe.Pointer(&out))),
                ((*Element384) (unsafe.Pointer(&x))),
                ((*Element384) (unsafe.Pointer(&y))),
                ((*Element384) (unsafe.Pointer(&mod))))
        default:
            panic("this case should have been checked before hitting here :)")
    }
}

func SubMod(limb_count int64, out []byte, x []byte, y []byte, mod []byte) {
    switch limb_count {
        case 4:
            SubMod256(
                ((*Element256) (unsafe.Pointer(&out))),
                ((*Element256) (unsafe.Pointer(&x))),
                ((*Element256) (unsafe.Pointer(&y))),
                ((*Element256) (unsafe.Pointer(&mod))))
        case 6:
            SubMod384(
                ((*Element384) (unsafe.Pointer(&out))),
                ((*Element384) (unsafe.Pointer(&x))),
                ((*Element384) (unsafe.Pointer(&y))),
                ((*Element384) (unsafe.Pointer(&mod))))
        default:
            panic("this case should have been checked before hitting here :)")
    }
}
