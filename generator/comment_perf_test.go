package generator

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/parser"
)

func benchFn() string {
	return `function f(a, b) {
  var x = a + b;
  if (x > 0) {
    return x * 2;
  }
  for (var i = 0; i < 10; i++) {
    x += i;
  }
  return x;
}
`
}

func BenchmarkGenerateSmallFreeSkip(b *testing.B) {
	p, err := parser.Parse(strings.Repeat(benchFn(), 20))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateWithOptions(p, Options{SkipComments: true})
	}
}

func BenchmarkGenerateSmallFree(b *testing.B) {
	p, err := parser.Parse(strings.Repeat(benchFn(), 20))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Generate(p)
	}
}

func BenchmarkGenerateWithComments(b *testing.B) {
	src := strings.Repeat("/* lead */ var x = /* mid */ 1; // trail\n", 500)
	p, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Generate(p)
	}
}

func BenchmarkGenerateMinifiedWithComments(b *testing.B) {
	src := strings.Repeat("/* @__PURE__ */ foo(); /* drop */ var x = 1; // trail\n", 500)
	p, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateMinified(p)
	}
}
