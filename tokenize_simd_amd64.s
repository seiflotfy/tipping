//go:build amd64 && !purego

#include "textflag.h"

// 16-byte constants for the classifier. hiBit[h] = 1<<h for h < 8, else 0.
DATA k9<>+0x00(SB)/8, $0x0909090909090909
DATA k9<>+0x08(SB)/8, $0x0909090909090909
GLOBL k9<>(SB), RODATA|NOPTR, $16

DATA k4<>+0x00(SB)/8, $0x0404040404040404
DATA k4<>+0x08(SB)/8, $0x0404040404040404
GLOBL k4<>(SB), RODATA|NOPTR, $16

DATA k20<>+0x00(SB)/8, $0x2020202020202020
DATA k20<>+0x08(SB)/8, $0x2020202020202020
GLOBL k20<>(SB), RODATA|NOPTR, $16

DATA ka<>+0x00(SB)/8, $0x6161616161616161
DATA ka<>+0x08(SB)/8, $0x6161616161616161
GLOBL ka<>(SB), RODATA|NOPTR, $16

DATA k25<>+0x00(SB)/8, $0x1919191919191919
DATA k25<>+0x08(SB)/8, $0x1919191919191919
GLOBL k25<>(SB), RODATA|NOPTR, $16

DATA k30<>+0x00(SB)/8, $0x3030303030303030
DATA k30<>+0x08(SB)/8, $0x3030303030303030
GLOBL k30<>(SB), RODATA|NOPTR, $16

DATA k0f<>+0x00(SB)/8, $0x0F0F0F0F0F0F0F0F
DATA k0f<>+0x08(SB)/8, $0x0F0F0F0F0F0F0F0F
GLOBL k0f<>(SB), RODATA|NOPTR, $16

DATA hibit<>+0x00(SB)/8, $0x8040201008040201
DATA hibit<>+0x08(SB)/8, $0x0000000000000000
GLOBL hibit<>(SB), RODATA|NOPTR, $16

DATA bit1<>+0x00(SB)/8, $0x0101010101010101
DATA bit1<>+0x08(SB)/8, $0x0101010101010101
GLOBL bit1<>(SB), RODATA|NOPTR, $16

DATA bit2<>+0x00(SB)/8, $0x0202020202020202
DATA bit2<>+0x08(SB)/8, $0x0202020202020202
GLOBL bit2<>(SB), RODATA|NOPTR, $16

DATA bit4<>+0x00(SB)/8, $0x0404040404040404
DATA bit4<>+0x08(SB)/8, $0x0404040404040404
GLOBL bit4<>(SB), RODATA|NOPTR, $16

DATA bit8<>+0x00(SB)/8, $0x0808080808080808
DATA bit8<>+0x08(SB)/8, $0x0808080808080808
GLOBL bit8<>(SB), RODATA|NOPTR, $16

DATA bit16<>+0x00(SB)/8, $0x1010101010101010
DATA bit16<>+0x08(SB)/8, $0x1010101010101010
GLOBL bit16<>(SB), RODATA|NOPTR, $16

// func classifyBytes(p *byte, n int, symLo *[16]byte, out *byte)
//
// SSE2 + PSHUFB (SSSE3) port of the arm64 NEON kernel; see
// tokenize_simd_arm64.s for the per-bit contract. Must stay byte-identical
// to classifyBytesGo. Requires n >= 16; the trailing partial chunk uses an
// overlapping load of the final 16 bytes.
TEXT ·classifyBytes(SB), NOSPLIT, $0-32
	MOVQ p+0(FP), SI
	MOVQ n+8(FP), CX
	MOVQ symLo+16(FP), DX
	MOVQ out+24(FP), DI

	// Overlap pointers for the tail; R8 = tail flag.
	LEAQ -16(SI)(CX*1), R9
	LEAQ -16(DI)(CX*1), R10
	MOVQ CX, R8
	ANDQ $15, R8
	SHRQ $4, CX

	MOVOU (DX), X8        // symLo shufti table
	MOVOU hibit<>(SB), X9
	MOVOU k0f<>(SB), X10
	MOVOU k20<>(SB), X11
	MOVOU k9<>(SB), X12
	PXOR  X13, X13        // zero

loop:
	MOVOU (SI), X0

	// space: (b-9) <= 4 || b == 0x20
	MOVOU  X0, X1
	PSUBB  X12, X1
	MOVOU  X1, X2
	PMINUB k4<>(SB), X2
	PCMPEQB X1, X2        // X2 = (b-9) <= 4
	MOVOU  X0, X3
	PCMPEQB X11, X3       // X3 = b == 0x20
	POR    X3, X2         // X2 = isSpace

	// letter: ((b|0x20) - 'a') <= 25
	MOVOU  X0, X1
	POR    X11, X1
	PSUBB  ka<>(SB), X1
	MOVOU  X1, X3
	PMINUB k25<>(SB), X3
	PCMPEQB X1, X3        // X3 = isLetter

	// digit: (b - '0') <= 9
	MOVOU  X0, X1
	PSUBB  k30<>(SB), X1
	MOVOU  X1, X4
	PMINUB X12, X4
	PCMPEQB X1, X4        // X4 = isDigit

	// symbol: symLo[b&0xF] & hiBit[(b>>4)&0xF] != 0
	MOVOU  X0, X1
	PAND   X10, X1        // lo nibble
	MOVOU  X0, X5
	PSRLW  $4, X5
	PAND   X10, X5        // hi nibble
	MOVOU  X8, X6
	PSHUFB X1, X6         // symLo[lo]
	MOVOU  X9, X7
	PSHUFB X5, X7         // hiBit[hi]
	PAND   X7, X6
	PCMPEQB X13, X6       // X6 = 0xFF where AND == 0
	PANDN  bit2<>(SB), X6 // X6 = symbol class bit where AND != 0

	// nonascii: signed b < 0
	MOVOU   X13, X5
	PCMPGTB X0, X5        // X5 = 0 > b (signed) = b >= 0x80

	// combine class bits
	PAND bit1<>(SB), X2
	PAND bit4<>(SB), X3
	PAND bit8<>(SB), X4
	PAND bit16<>(SB), X5
	POR  X3, X2
	POR  X4, X2
	POR  X5, X2
	POR  X6, X2

	MOVOU X2, (DI)
	ADDQ  $16, SI
	ADDQ  $16, DI
	SUBQ  $1, CX
	JNE   loop

	TESTQ R8, R8
	JZ    done
	MOVQ  R9, SI
	MOVQ  R10, DI
	MOVQ  $1, CX
	XORQ  R8, R8
	JMP   loop

done:
	RET

// func cpuHasSSSE3() bool
TEXT ·cpuHasSSSE3(SB), NOSPLIT, $0-1
	MOVQ BX, R11          // CPUID clobbers BX
	MOVL $1, AX
	XORL CX, CX
	CPUID
	MOVQ R11, BX
	SHRL $9, CX           // SSSE3 = ECX bit 9
	ANDL $1, CX
	MOVB CX, ret+0(FP)
	RET
