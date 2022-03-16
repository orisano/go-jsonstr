package jsonstr

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnescapeAVX(t *testing.T) {
	src := []byte(`a\r\n"` + strings.Repeat(" ", 31))
	dst := make([]byte, len(src))
	n := UnescapeAVX(dst, src)
	dst = dst[:n]
	if string(dst) != "a\r\n" {
		t.Errorf("unexpected result: %q", string(dst))
	}
}

func TestUnescapeSSE(t *testing.T) {
	src := []byte(`a\r\n"` + strings.Repeat(" ", 16))
	dst := make([]byte, len(src))
	n := UnescapeSSE(dst, src)
	dst = dst[:n]
	if string(dst) != "a\r\n" {
		t.Errorf("unexpected result: %q", string(dst))
	}
}

func TestUnescapeNaive(t *testing.T) {
	src := []byte(`a\r\n"` + strings.Repeat(" ", 16))
	dst := make([]byte, len(src))
	n := UnescapeNaive(dst, src)
	dst = dst[:n]
	if string(dst) != "a\r\n" {
		t.Errorf("unexpected result: %q", string(dst))
	}
}

func BenchmarkUnescape(b *testing.B) {
	str := strings.Repeat(`abcd"`, 10000)
	in, _ := json.Marshal(str)
	padded := append(in, make([]byte, 31)...)
	src := padded[1:]
	out := make([]byte, len(in))
	b.Run("Std", func(b *testing.B) {
		s := string(out)
		for i := 0; i < b.N; i++ {
			json.Unmarshal(in, &s)
		}
	})
	b.Run("Go", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n := Unescape(out, src)
			_ = out[:n]
		}
	})
	b.Run("Naive", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n := UnescapeNaive(out, src)
			_ = out[:n]
		}
	})
	b.Run("AVX", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n := UnescapeAVX(out, src)
			_ = out[:n]
		}
	})
	b.Run("SSE", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n := UnescapeSSE(out, src)
			_ = out[:n]
		}
	})
}
