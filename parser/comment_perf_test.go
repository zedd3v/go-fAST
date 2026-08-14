package parser

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/parser/scanner"
	"github.com/t14raptor/go-fast/parser/scanner/token"
)

func benchLargeFree() string {
	return strings.Repeat(benchCommentFree, 200)
}

func benchScan(b *testing.B, src string) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		var err error
		s := scanner.NewScanner(src, &err)
		for {
			s.Next()
			if s.Token.Kind == token.Eof {
				break
			}
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchParse(b *testing.B, src string) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanSmallFree(b *testing.B) { benchScan(b, benchCommentFree) }
func BenchmarkScanLargeFree(b *testing.B) { benchScan(b, benchLargeFree()) }
func BenchmarkScan10kComments(b *testing.B) {
	benchScan(b, bench10kComments)
}

func benchParseOpts(b *testing.B, src string, opts Options) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, err := ParseWithOptions(src, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSmallFree(b *testing.B) { benchParse(b, benchCommentFree) }
func BenchmarkParseLargeFree(b *testing.B) { benchParse(b, benchLargeFree()) }
func BenchmarkParseLargeFreeSkip(b *testing.B) {
	benchParseOpts(b, benchLargeFree(), Options{SkipComments: true})
}
func BenchmarkParse10kDump(b *testing.B) { benchParse(b, bench10kComments) }
func BenchmarkParse10kDumpSkip(b *testing.B) {
	benchParseOpts(b, bench10kComments, Options{SkipComments: true})
}

func BenchmarkScanLargeFreeSkip(b *testing.B) {
	src := benchLargeFree()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		var err error
		s := scanner.NewScanner(src, &err)
		s.CollectComments(false)
		for {
			s.Next()
			if s.Token.Kind == token.Eof {
				break
			}
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseNormalComments(b *testing.B) {
	src := strings.Repeat("/* mid */ var x = 1; // trail\n", 2000)
	benchParse(b, src)
}
