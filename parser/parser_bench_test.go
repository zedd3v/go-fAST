package parser

import (
	"strconv"
	"strings"
	"testing"
)

var benchCommentFree = `function f(a, b) {
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

var bench10kComments = build10kComments()

func build10kComments() string {
	var b strings.Builder
	b.Grow(10000 * 32)
	for i := 0; i < 10000; i++ {
		b.WriteString("/* 1 pc=2 dk=3 */ var v")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" = 1;\n")
	}
	return b.String()
}

func BenchmarkParseCommentFree(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(benchCommentFree); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse10kComments(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(bench10kComments); err != nil {
			b.Fatal(err)
		}
	}
}
