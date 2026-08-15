//go:build arm64 && !purego

#include "textflag.h"

// func classifyBytes(p *byte, n int, symLo *[16]byte, out *byte)
//
// Requires n >= 16. For each input byte b emits a class byte:
//   bit0 space    : b == ' ' || '\t' <= b <= '\r'
//   bit1 symbol   : shufti lookup — symLo[b&0xF] has bit b>>4 (ASCII only)
//   bit2 letter   : 'a' <= b|0x20 <= 'z'
//   bit3 digit    : '0' <= b <= '9'
//   bit4 nonascii : b >= 0x80
//
// Unsigned "x <= K" is computed as UMIN(x, K) == x since the assembler has no
// vector unsigned compares. A trailing partial chunk is classified with an
// overlapping load of the final 16 bytes; overlapped bytes are rewritten with
// identical values. Must stay byte-identical to classifyBytesGo.
TEXT ·classifyBytes(SB), NOSPLIT, $0-32
	MOVD p+0(FP), R0
	MOVD n+8(FP), R1
	MOVD symLo+16(FP), R2
	MOVD out+24(FP), R3

	// R5/R6: overlapping pointers for the final 16 bytes; R7: tail flag.
	ADD  R1, R0, R5
	SUB  $16, R5
	ADD  R1, R3, R6
	SUB  $16, R6
	AND  $15, R1, R7
	LSR  $4, R1, R1

	// Constants.
	MOVD $0x20, R4
	VDUP R4, V16.B16              // 0x20: ' ' compare and case-fold OR
	MOVD $9, R4
	VDUP R4, V17.B16              // 9: space offset ('\t') and digit range max
	MOVD $4, R4
	VDUP R4, V18.B16              // 4: space range max ('\r'-'\t') and letter bit
	MOVD $0x61, R4
	VDUP R4, V19.B16              // 'a'
	MOVD $25, R4
	VDUP R4, V20.B16              // 'z'-'a'
	MOVD $0x30, R4
	VDUP R4, V21.B16              // '0'
	MOVD $0x0F, R4
	VDUP R4, V23.B16              // low-nibble mask
	VLD1 (R2), [V24.B16]          // symLo shufti table
	VMOVQ $0x8040201008040201, $0x0000000000000000, V25 // hiBit[h] = 1<<h for h<8, else 0
	MOVD $0x80, R4
	VDUP R4, V26.B16              // non-ASCII bit test
	MOVD $1, R4
	VDUP R4, V27.B16              // space class bit
	MOVD $2, R4
	VDUP R4, V28.B16              // symbol class bit
	MOVD $8, R4
	VDUP R4, V30.B16              // digit class bit
	MOVD $16, R4
	VDUP R4, V31.B16              // nonascii class bit

loop:
	VLD1.P 16(R0), [V0.B16]

	// space: (b-9) <= 4 || b == 0x20
	VSUB  V17.B16, V0.B16, V1.B16
	VUMIN V18.B16, V1.B16, V2.B16
	VCMEQ V1.B16, V2.B16, V2.B16
	VCMEQ V16.B16, V0.B16, V3.B16
	VORR  V3.B16, V2.B16, V2.B16  // V2 = isSpace

	// letter: ((b|0x20) - 'a') <= 25
	VORR  V16.B16, V0.B16, V1.B16
	VSUB  V19.B16, V1.B16, V1.B16
	VUMIN V20.B16, V1.B16, V3.B16
	VCMEQ V1.B16, V3.B16, V3.B16  // V3 = isLetter

	// digit: (b - '0') <= 9
	VSUB  V21.B16, V0.B16, V1.B16
	VUMIN V17.B16, V1.B16, V4.B16
	VCMEQ V1.B16, V4.B16, V4.B16  // V4 = isDigit

	// symbol: symLo[b&0xF] & hiBit[b>>4] != 0
	VAND   V23.B16, V0.B16, V1.B16
	VUSHR  $4, V0.B16, V5.B16
	VTBL   V1.B16, [V24.B16], V1.B16
	VTBL   V5.B16, [V25.B16], V5.B16
	VCMTST V5.B16, V1.B16, V5.B16 // V5 = isSymbol

	// nonascii: b & 0x80 != 0
	VCMTST V26.B16, V0.B16, V6.B16

	// Combine class bits.
	VAND V27.B16, V2.B16, V2.B16
	VAND V28.B16, V5.B16, V5.B16
	VAND V18.B16, V3.B16, V3.B16
	VAND V30.B16, V4.B16, V4.B16
	VAND V31.B16, V6.B16, V6.B16
	VORR V5.B16, V2.B16, V2.B16
	VORR V4.B16, V3.B16, V3.B16
	VORR V6.B16, V2.B16, V2.B16
	VORR V3.B16, V2.B16, V2.B16

	VST1.P [V2.B16], 16(R3)
	SUB  $1, R1
	CBNZ R1, loop

	CBZ  R7, done
	MOVD R5, R0
	MOVD R6, R3
	MOVD $1, R1
	MOVD ZR, R7
	B    loop

done:
	RET
