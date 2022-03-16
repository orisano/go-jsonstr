package jsonstr

import (
	"unsafe"
)

var escapeMap = [256]byte{
	'"':  '"',
	'/':  '/',
	'\\': '\\',
	'b':  '\b',
	'f':  '\f',
	'n':  '\n',
	'r':  '\r',
	't':  '\t',
}

var digitToVal32 [4][256]uint32

func init() {
	for i := 0; i < 4; i++ {
		for _, c := range []byte("0123456789ABCDEFabcdef") {
			digitToVal32[i][c] = hex(c) << uint32(i*4)
		}
	}
}

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

func get(ptr unsafe.Pointer, offset int64) byte {
	return *(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(offset)))
}

func put(ptr unsafe.Pointer, offset int64, b byte) {
	*(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(offset))) = b
}

func Unescape(dst []byte, src []byte) int {
	type sliceHeader struct {
		data unsafe.Pointer
		len  int
		cap  int
	}
	dstp := (*sliceHeader)(unsafe.Pointer(&dst)).data
	srcp := (*sliceHeader)(unsafe.Pointer(&src)).data
	for {
		c := *(*byte)(srcp)
		*(*byte)(dstp) = c
		if c == '\\' {
			escaped := get(srcp, 1)
			if escaped != 'u' {
				*(*byte)(dstp) = escapeMap[escaped]
				srcp = unsafe.Pointer(uintptr(srcp) + 2)
				dstp = unsafe.Pointer(uintptr(dstp) + 1)
			} else {
				v1 := digitToVal32[3][get(srcp, 2)]
				v2 := digitToVal32[2][get(srcp, 3)]
				v3 := digitToVal32[1][get(srcp, 4)]
				v4 := digitToVal32[0][get(srcp, 5)]
				srcp = unsafe.Pointer(uintptr(srcp) + 6)
				cp := v1 | v2 | v3 | v4
				if cp <= 0x7f {
					*(*byte)(dstp) = byte(cp)
					dstp = unsafe.Pointer(uintptr(dstp) + 1)
				} else if cp <= 0x7ff {
					put(dstp, 0, byte((cp>>6)+192))
					put(dstp, 1, byte((cp&63)+128))
					dstp = unsafe.Pointer(uintptr(dstp) + 2)
				} else if cp <= 0xffff {
					put(dstp, 0, byte((cp>>12)+224))
					put(dstp, 1, byte(((cp>>6)&63)+128))
					put(dstp, 2, byte((cp&63)+128))
					dstp = unsafe.Pointer(uintptr(dstp) + 3)
				} else if cp <= 0x10FFFF {
					put(dstp, 0, byte((cp>>18)+240))
					put(dstp, 1, byte(((cp>>12)&63)+128))
					put(dstp, 2, byte(((cp>>6)&63)+128))
					put(dstp, 3, byte((cp&63)+128))
					dstp = unsafe.Pointer(uintptr(dstp) + 4)
				}
			}
		} else if c != '"' {
			srcp = unsafe.Pointer(uintptr(srcp) + 1)
			dstp = unsafe.Pointer(uintptr(dstp) + 1)
		} else {
			return int(uintptr(dstp) - uintptr((*sliceHeader)(unsafe.Pointer(&dst)).data))
		}
	}
}
