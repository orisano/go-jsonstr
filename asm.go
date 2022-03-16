//go:build ignore
// +build ignore

package main

import (
	"strings"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

func hex(b byte) uint32 {
	if '0' <= b && b <= '9' {
		return uint32(b - '0')
	}
	if 'a' <= b && b <= 'f' {
		return uint32(b-'a') + 10
	}
	if 'A' <= b && b <= 'F' {
		return uint32(b-'A') + 10
	}
	return 0xffffffff
}

type Globals struct {
	quotes       Mem
	backslashes  Mem
	escapeMap    Mem
	digitToVal32 Mem
}

func (g *Globals) init() {
	g.quotes = GLOBL("quotes", RODATA|NOPTR)
	DATA(0, String(strings.Repeat(`"`, 32)))
	g.backslashes = GLOBL("backslashes", RODATA|NOPTR)
	DATA(0, String(strings.Repeat(`\`, 32)))
	g.escapeMap = GLOBL("escapeMap", RODATA|NOPTR)
	for i, b := range [256]byte{
		'"':  '"',
		'/':  '/',
		'\\': '\\',
		'b':  '\b',
		'f':  '\f',
		'n':  '\n',
		'r':  '\r',
		't':  '\t',
	} {
		DATA(i, U8(b))
	}
	var digitToVal32 [886]uint32
	for i := range digitToVal32 {
		digitToVal32[i] = 0xffffffff
	}
	for i := 0; i < 4; i++ {
		for _, c := range []byte("0123456789ABCDEFabcdef") {
			digitToVal32[int(c)+i*210] = hex(c) << uint32(i*4)
		}
	}
	g.digitToVal32 = GLOBL("digitToVal32", RODATA|NOPTR)
	for i, u := range digitToVal32 {
		DATA(i*4, U32(u))
	}
}

func utf8(mem Mem, cp reg.Register, shift, and, add uint64) {
	x := GP32()
	MOVL(cp, x)
	if shift != 0 {
		SHRL(Imm(shift), x)
	}
	if and != 0 {
		ANDL(Imm(and), x)
	}
	if add == 0x80 {
		SUBL(U8(-0x80), x)
	} else {
		ADDL(Imm(add), x)
	}
	MOVB(x.As8(), mem)
}

func addq(imm uint64, r64 reg.Register) {
	if imm == 1 {
		INCQ(r64)
	} else {
		ADDQ(Imm(imm), r64)
	}
}

func main() {
	var globals Globals
	globals.init()

	TEXT("UnescapeAVX", NOSPLIT, "func(dst, src []byte) int")
	buildEscapeFunc(&backslashAndQuoteAVX{}, &globals)

	TEXT("UnescapeSSE", NOSPLIT, "func(dst, src []byte) int")
	buildEscapeFunc(&backslashAndQuoteSSE{}, &globals)

	TEXT("UnescapeNaive", NOSPLIT, "func(dst, src []byte) int")
	buildEscapeFunc(&backslashAndQuoteNaive{}, &globals)
	Generate()
}

type backslashAndQuote interface {
	onInit(globals *Globals)
	onEnd()
	copyAndFind(src, dst Mem)
	hasQuoteFirst(ref LabelRef)
	hasBackslash(ref LabelRef)
	forwardToQuote(m Mem)
	forwardToBackslash(src, dst Mem)
	processBytes() uint64
}

type backslashAndQuoteAVX struct {
	bsMask    reg.VecVirtual
	quoteMask reg.VecVirtual
	bsBits    reg.GPVirtual
	quoteBits reg.GPVirtual
}

func (baq *backslashAndQuoteAVX) onInit(globals *Globals) {
	baq.bsMask = YMM()
	VMOVDQU(globals.backslashes, baq.bsMask)
	baq.quoteMask = YMM()
	VMOVDQU(globals.quotes, baq.quoteMask)
}

func (baq *backslashAndQuoteAVX) onEnd() {
	VZEROUPPER()
}

func (baq *backslashAndQuoteAVX) copyAndFind(src, dst Mem) {
	src32 := YMM()
	VMOVDQU(src, src32)
	VMOVDQU(src32, dst)
	quoteCmp := YMM()
	VPCMPEQB(baq.quoteMask, src32, quoteCmp)
	baq.quoteBits = GP32()
	VPMOVMSKB(quoteCmp, baq.quoteBits)
	bsCmp := YMM()
	VPCMPEQB(baq.bsMask, src32, bsCmp)
	baq.bsBits = GP32()
	VPMOVMSKB(bsCmp, baq.bsBits)
}

func (baq *backslashAndQuoteAVX) hasQuoteFirst(ref LabelRef) {
	hasQuoteFirst := GP32()
	MOVL(baq.bsBits, hasQuoteFirst)
	DECL(hasQuoteFirst)
	TESTL(hasQuoteFirst, baq.quoteBits)
	JNE(ref)
}

func (baq *backslashAndQuoteAVX) hasBackslash(ref LabelRef) {
	hasBackslash := GP32()
	MOVL(baq.quoteBits, hasBackslash)
	DECL(hasBackslash)
	TESTL(hasBackslash, baq.bsBits)
	JNE(ref)
}

func (baq *backslashAndQuoteAVX) forwardToQuote(m Mem) {
	quoteDist := GP64()
	XORQ(quoteDist, quoteDist)
	BSFL(baq.quoteBits, quoteDist.As32())
	ADDQ(quoteDist, m.Base)
}

func (baq *backslashAndQuoteAVX) forwardToBackslash(src, dst Mem) {
	bsDist := GP64()
	XORQ(bsDist, bsDist)
	BSFL(baq.bsBits, bsDist.As32())
	ADDQ(bsDist, src.Base)
	ADDQ(bsDist, dst.Base)
}

func (baq *backslashAndQuoteAVX) processBytes() uint64 {
	return 32
}

type backslashAndQuoteSSE struct {
	bsMask    reg.VecVirtual
	quoteMask reg.VecVirtual
	bsBits    reg.GPVirtual
	quoteBits reg.GPVirtual
}

func (baq *backslashAndQuoteSSE) onInit(globals *Globals) {
	baq.bsMask = XMM()
	MOVOU(globals.backslashes, baq.bsMask)
	baq.quoteMask = XMM()
	MOVOU(globals.quotes, baq.quoteMask)
}

func (baq *backslashAndQuoteSSE) onEnd() {}

func (baq *backslashAndQuoteSSE) copyAndFind(src, dst Mem) {
	src16 := XMM()
	MOVOU(src, src16)
	MOVOU(src16, dst)
	quoteCmp := XMM()
	MOVQ(src16, quoteCmp)
	PCMPEQB(baq.quoteMask, quoteCmp)
	baq.quoteBits = GP32()
	PMOVMSKB(quoteCmp, baq.quoteBits)
	bsCmp := src16
	PCMPEQB(baq.bsMask, bsCmp)
	baq.bsBits = GP32()
	PMOVMSKB(bsCmp, baq.bsBits)
}

func (baq *backslashAndQuoteSSE) hasQuoteFirst(ref LabelRef) {
	hasQuoteFirst := GP32()
	MOVL(baq.bsBits, hasQuoteFirst)
	DECL(hasQuoteFirst)
	TESTL(hasQuoteFirst, baq.quoteBits)
	JNE(ref)
}

func (baq *backslashAndQuoteSSE) hasBackslash(ref LabelRef) {
	hasBackslash := GP32()
	MOVL(baq.quoteBits, hasBackslash)
	DECL(hasBackslash)
	TESTL(hasBackslash, baq.bsBits)
	JNE(ref)
}

func (baq *backslashAndQuoteSSE) forwardToQuote(m Mem) {
	quoteDist := GP64()
	XORQ(quoteDist, quoteDist)
	BSFL(baq.quoteBits, quoteDist.As32())
	ADDQ(quoteDist, m.Base)
}

func (baq *backslashAndQuoteSSE) forwardToBackslash(src, dst Mem) {
	bsDist := GP64()
	XORQ(bsDist, bsDist)
	BSFL(baq.bsBits, bsDist.As32())
	ADDQ(bsDist, src.Base)
	ADDQ(bsDist, dst.Base)
}

func (baq *backslashAndQuoteSSE) processBytes() uint64 {
	return 16
}

type backslashAndQuoteNaive struct {
	c reg.GPVirtual
}

func (baq *backslashAndQuoteNaive) onInit(globals *Globals) {}

func (baq *backslashAndQuoteNaive) onEnd() {}

func (baq *backslashAndQuoteNaive) copyAndFind(src, dst Mem) {
	baq.c = GP64()
	MOVBQZX(src, baq.c)
	MOVB(baq.c.As8(), dst)
}

func (baq *backslashAndQuoteNaive) hasQuoteFirst(ref LabelRef) {
	CMPL(baq.c.As32(), U8('"'))
	JE(ref)
}

func (baq *backslashAndQuoteNaive) hasBackslash(ref LabelRef) {
	CMPL(baq.c.As32(), U8('\\'))
	JE(ref)
}

func (baq *backslashAndQuoteNaive) forwardToQuote(m Mem) {}

func (baq *backslashAndQuoteNaive) forwardToBackslash(src, dst Mem) {}

func (baq *backslashAndQuoteNaive) processBytes() uint64 {
	return 1
}

func buildEscapeFunc(baq backslashAndQuote, globals *Globals) {
	dst := Mem{Base: Load(Param("dst").Base(), GP64())}
	src := Mem{Base: Load(Param("src").Base(), GP64())}

	baq.onInit(globals)

	escapeMap := Mem{Base: GP64()}
	LEAQ(globals.escapeMap, escapeMap.Base)

	digitToVal32 := Mem{Base: GP64()}
	LEAQ(globals.digitToVal32, digitToVal32.Base)

	Label("loop")
	baq.copyAndFind(src, dst)
	baq.hasQuoteFirst("done")
	baq.hasBackslash("escaped")
	addq(baq.processBytes(), src.Base)
	addq(baq.processBytes(), dst.Base)
	JMP(LabelRef("loop"))

	Label("escaped")
	baq.forwardToBackslash(src, dst)
	escapeChar := GP64()
	MOVBQZX(src.Offset(1), escapeChar)
	CMPL(escapeChar.As32(), U8('u'))
	JE(LabelRef("unicode"))

	escapeResult := GP64()
	MOVBQZX(escapeMap.Idx(escapeChar, 1), escapeResult)
	MOVB(escapeResult.As8(), dst)
	addq(2, src.Base)
	addq(1, dst.Base)
	JMP(LabelRef("loop"))

	Label("unicode")
	cp := GP32()
	for i := 0; i < 4; i++ {
		h := GP64()
		XORQ(h, h)
		MOVB(src.Offset(i+2), h.As8())
		v := GP32()
		MOVL(digitToVal32.Offset(4*(3-i)*210).Idx(h, 4), v)
		ORL(v, cp)
	}
	addq(6, src.Base)

	CMPL(cp, U8(0x7f))
	JLE(LabelRef("unicode1"))
	CMPL(cp, U32(0x7ff))
	JLE(LabelRef("unicode2"))
	CMPL(cp, U32(0xffff))
	JLE(LabelRef("unicode3"))
	CMPL(cp, U32(0x10FFFF))
	JB(LabelRef("loop"))

	Label("unicode4")
	utf8(dst.Offset(0), cp, 18, 0, 240)
	utf8(dst.Offset(1), cp, 12, 63, 128)
	utf8(dst.Offset(2), cp, 6, 63, 128)
	utf8(dst.Offset(3), cp, 0, 63, 128)
	addq(4, dst.Base)
	JMP(LabelRef("loop"))

	Label("unicode3")
	utf8(dst.Offset(0), cp, 12, 0, 224)
	utf8(dst.Offset(1), cp, 6, 63, 128)
	utf8(dst.Offset(2), cp, 0, 63, 128)
	addq(3, dst.Base)
	JMP(LabelRef("loop"))

	Label("unicode2")
	utf8(dst.Offset(0), cp, 6, 0, 192)
	utf8(dst.Offset(1), cp, 0, 63, 128)
	addq(2, dst.Base)
	JMP(LabelRef("loop"))

	Label("unicode1")
	MOVB(cp.As8(), dst)
	INCQ(dst.Base)
	JMP(LabelRef("loop"))

	Label("done")
	baq.forwardToQuote(dst)
	SUBQ(Load(Param("dst").Base(), GP64()), dst.Base)
	Store(dst.Base, ReturnIndex(0))
	baq.onEnd()
	RET()
}
