package scanner

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/ast"
	"github.com/t14raptor/go-fast/parser/scanner/token"
)

func collectComments(src string) ([]ast.Comment, error) {
	var err error
	s := NewScanner(src, &err)
	for {
		s.Next()
		if s.Token.Kind == token.Eof {
			break
		}
	}
	return s.TakeComments(), err
}

func TestNewScannerInitsTrivia(t *testing.T) {
	var err error
	s := NewScanner("// lead\nvar x", &err)
	if !s.trivia.sawNewline || !s.trivia.sawNewlineForComment {
		t.Fatal("NewScanner must start trivia with sawNewline true")
	}
}

func TestTriviaClassification(t *testing.T) {
	type want struct {
		text     string
		pos      ast.CommentPosition
		content  ast.CommentContent
		attach   string // unique token text; AttachedTo == index of this string
		attachAt int    // used when attach == ""
		useIdx   bool
	}

	cases := []struct {
		name string
		src  string
		want []want
	}{
		{
			name: "leading block",
			src:  "/* lead */ var x",
			want: []want{{
				text:    "/* lead */",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "var",
			}},
		},
		{
			name: "trailing line",
			src:  "var x = 1 // trail",
			want: []want{{
				text:    "// trail",
				pos:     ast.CommentTrailing,
				content: ast.ContentNone,
				attach:  "1",
			}},
		},
		{
			name: "eq exception",
			src:  "let y = // not\n10",
			want: []want{{
				text:    "// not",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "10",
			}},
		},
		{
			name: "paren exception",
			src:  "foo( // not\n1)",
			want: []want{{
				text:    "// not",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "1",
			}},
		},
		{
			name: "colon exception",
			src:  "v = c ? a : // not\nb",
			want: []want{{
				text:    "// not",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "b",
			}},
		},
		{
			name: "legal stay-leading",
			src:  "foo();//! @license\nfunction bar(){}",
			want: []want{{
				text:    "//! @license",
				pos:     ast.CommentLeading,
				content: ast.ContentLegal,
				attach:  "function",
			}},
		},
		{
			name: "pure stay-leading",
			src:  "foo();/* @__PURE__ */bar()",
			want: []want{{
				text:    "/* @__PURE__ */",
				pos:     ast.CommentLeading,
				content: ast.ContentPure,
				attach:  "bar",
			}},
		},
		{
			name: "dump stay-leading",
			src:  "foo();/* 1 pc=2 dk=3 */var x",
			want: []want{{
				text:    "/* 1 pc=2 dk=3 */",
				pos:     ast.CommentLeading,
				content: ast.ContentDumpMeta,
				attach:  "var",
			}},
		},
		{
			name: "dump meta leading",
			src:  "/* 7355685938729369933 pc=114796 dk=5 */ var v67",
			want: []want{{
				text:    "/* 7355685938729369933 pc=114796 dk=5 */",
				pos:     ast.CommentLeading,
				content: ast.ContentDumpMeta,
				attach:  "var",
			}},
		},
		{
			name: "leading line",
			src:  "// line\nvar x",
			want: []want{{
				text:    "// line",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "var",
			}},
		},
		{
			name: "inner mid",
			src:  "var v67 = /* mid */ heap",
			want: []want{{
				text:    "/* mid */",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "heap",
			}},
		},
		{
			name: "inner name",
			src:  "var /* name */ v67",
			want: []want{{
				text:    "/* name */",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "v67",
			}},
		},
		{
			name: "two leading",
			src:  "/* a */ /* b */ var x",
			want: []want{
				{text: "/* a */", pos: ast.CommentLeading, content: ast.ContentNone, attach: "var"},
				{text: "/* b */", pos: ast.CommentLeading, content: ast.ContentNone, attach: "var"},
			},
		},
		{
			name: "async inner",
			src:  "async /* x */ function f(){}",
			want: []want{{
				text:    "/* x */",
				pos:     ast.CommentLeading,
				content: ast.ContentNone,
				attach:  "function",
			}},
		},
		{
			name: "trailing line after pending block",
			src:  "foo() /* a */ // b",
			want: []want{
				{text: "/* a */", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ")"},
				{text: "// b", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ")"},
			},
		},
		{
			name: "trailing line after pending blocks",
			src:  "foo() /* a */ /* b */ // c",
			want: []want{
				{text: "/* a */", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ")"},
				{text: "/* b */", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ")"},
				{text: "// c", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ")"},
			},
		},
		{
			name: "trailing line after pending block and semicolon",
			src:  "foo(); /* a */ // b",
			want: []want{
				{text: "/* a */", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ";"},
				{text: "// b", pos: ast.CommentTrailing, content: ast.ContentNone, attach: ";"},
			},
		},
		{
			name: "hashbang skipped",
			src:  "#!/usr/bin/env node\nvar x",
			want: nil,
		},
		{
			name: "empty block",
			src:  "/**/var x",
			want: []want{{text: "/**/", pos: ast.CommentLeading, content: ast.ContentNone, attach: "var"}},
		},
		{
			name: "empty line",
			src:  "//\nvar x",
			want: []want{{text: "//", pos: ast.CommentLeading, content: ast.ContentNone, attach: "var"}},
		},
		{
			name: "trailing same-line block before eof",
			src:  "foo() /* t */",
			want: []want{{text: "/* t */", pos: ast.CommentLeading, content: ast.ContentNone, useIdx: true, attachAt: 13}},
		},
		{
			name: "file is only a comment",
			src:  "/* only */",
			want: []want{{text: "/* only */", pos: ast.CommentLeading, content: ast.ContentNone, useIdx: true, attachAt: 10}},
		},
		{
			name: "eof line comment",
			src:  "var x//eof",
			want: []want{{text: "//eof", pos: ast.CommentTrailing, content: ast.ContentNone, attach: "x"}},
		},
		{
			name: "crlf line comment",
			src:  "a // c\r\nb",
			want: []want{{text: "// c", pos: ast.CommentTrailing, content: ast.ContentNone, attach: "a"}},
		},
		{
			name: "block contains line markers",
			src:  "/* // not a line\n */ var x",
			want: []want{{text: "/* // not a line\n */", pos: ast.CommentLeading, content: ast.ContentNone, attach: "var"}},
		},
		{
			name: "first star-slash wins",
			src:  "/* a */ + x",
			want: []want{{text: "/* a */", pos: ast.CommentLeading, content: ast.ContentNone, attach: "+"}},
		},
		{
			name: "nosideeffects hash",
			src:  "/* #__NO_SIDE_EFFECTS__ */ function f(){}",
			want: []want{{text: "/* #__NO_SIDE_EFFECTS__ */", pos: ast.CommentLeading, content: ast.ContentNoSideEffects, attach: "function"}},
		},
		{
			name: "pure with spaces",
			src:  "/*  @__PURE__  */ foo()",
			want: []want{{text: "/*  @__PURE__  */", pos: ast.CommentLeading, content: ast.ContentPure, attach: "foo"}},
		},
		{
			name: "between statements",
			src:  "a();\n/* mid */\nb();",
			want: []want{{text: "/* mid */", pos: ast.CommentLeading, content: ast.ContentNone, attach: "b"}},
		},
		{
			name: "call post comments",
			src:  "test(123/*post: 9*/, 456/*post: 10*/)",
			want: []want{
				{text: "/*post: 9*/", pos: ast.CommentLeading, content: ast.ContentNone, attach: ","},
				{text: "/*post: 10*/", pos: ast.CommentLeading, content: ast.ContentNone, attach: ")"},
			},
		},
		{
			name: "paren pure",
			src:  "/*#__PURE__*/ (console.log('s'))",
			want: []want{{text: "/*#__PURE__*/", pos: ast.CommentLeading, content: ast.ContentPure, attach: "("}},
		},
		{
			name: "switch fallthrough",
			src:  "switch (1) {\n    case 2:\n        3;\n    // 4\n}",
			want: []want{{text: "// 4", pos: ast.CommentLeading, content: ast.ContentNone, attach: "}"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectComments(tc.src)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d; want %d (%#v)", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				c := got[i]
				if text := c.Text(tc.src); text != w.text {
					t.Errorf("[%d] text = %q; want %q", i, text, w.text)
				}
				if c.Position != w.pos {
					t.Errorf("[%d] pos = %v; want %v", i, c.Position, w.pos)
				}
				if c.Content != w.content {
					t.Errorf("[%d] content = %v; want %v", i, c.Content, w.content)
				}
				attach := w.attachAt
				if !w.useIdx {
					attach = strings.Index(tc.src, w.attach)
					if attach < 0 {
						t.Fatalf("[%d] attach token %q not in source", i, w.attach)
					}
				}
				if int(c.AttachedTo) != attach {
					t.Errorf("[%d] AttachedTo = %d; want %d", i, c.AttachedTo, attach)
				}
			}
		})
	}
}

func TestTriviaRewindNoDup(t *testing.T) {
	src := "a /* c */ b"
	var err error
	s := NewScanner(src, &err)
	s.Next() // a
	cp := s.Checkpoint()
	s.Next() // b
	s.Rewind(cp)
	s.Next() // b again
	s.Next() // eof
	got := s.TakeComments()
	if len(got) != 1 {
		t.Fatalf("len=%d; want 1", len(got))
	}
	if got[0].Text(src) != "/* c */" {
		t.Fatalf("text = %q", got[0].Text(src))
	}
}

func TestTriviaDedupWithoutTruncate(t *testing.T) {
	src := "a /* c */ b"
	var err error
	s := NewScanner(src, &err)
	s.Next()
	s.Next()
	// Re-insert the same span without rewind truncate.
	start := ast.Idx(strings.Index(src, "/* c */"))
	s.trivia.addComment(start, start+ast.Idx(len("/* c */")), ast.CommentSingleLineBlock, s.src)
	if len(s.trivia.comments) != 1 {
		t.Fatalf("len=%d; want 1 after dedup", len(s.trivia.comments))
	}
}

func TestTriviaPeekDoesNotRelex(t *testing.T) {
	src := "a /* c */ b"
	var err error
	s := NewScanner(src, &err)
	s.Next()
	cur := s.Token
	pk := s.Peek()
	if s.Token != cur {
		t.Fatal("Peek changed current Token")
	}
	if pk.Kind != token.Identifier || s.src.Slice(pk.Idx0, pk.Idx1) != "b" {
		t.Fatalf("peek = %v %q", pk.Kind, s.src.Slice(pk.Idx0, pk.Idx1))
	}
	if s.Peek() != pk {
		t.Fatal("second Peek rescaned")
	}
	s.Next()
	if s.Token != pk {
		t.Fatal("Next did not consume peek")
	}
	s.Next()
	got := s.TakeComments()
	if len(got) != 1 {
		t.Fatalf("len=%d; want 1", len(got))
	}
}

func TestTriviaUnterminatedBlock(t *testing.T) {
	src := "/* oops"
	got, err := collectComments(src)
	if err == nil {
		t.Fatal("want unterminated error")
	}
	if len(got) != 1 {
		t.Fatalf("len=%d; want 1", len(got))
	}
	if got[0].Start != 0 || int(got[0].End) != len(src) {
		t.Fatalf("span = [%d,%d); want [0,%d)", got[0].Start, got[0].End, len(src))
	}
}

func TestTriviaASILineComment(t *testing.T) {
	src := "a // c\nb"
	var err error
	s := NewScanner(src, &err)
	s.Next()
	if s.Token.Kind != token.Identifier || s.src.Slice(s.Token.Idx0, s.Token.Idx1) != "a" {
		t.Fatalf("first = %q", s.src.Slice(s.Token.Idx0, s.Token.Idx1))
	}
	s.Next()
	if !s.Token.OnNewLine {
		t.Fatal("line comment must leave terminator for ASI")
	}
	if s.src.Slice(s.Token.Idx0, s.Token.Idx1) != "b" {
		t.Fatalf("second = %q", s.src.Slice(s.Token.Idx0, s.Token.Idx1))
	}
}

func TestTriviaASIBlockComment(t *testing.T) {
	src := "a /*\n*/ b"
	var err error
	s := NewScanner(src, &err)
	s.Next()
	s.Next()
	if !s.Token.OnNewLine {
		t.Fatal("block comment with newline must set OnNewLine")
	}
	if s.src.Slice(s.Token.Idx0, s.Token.Idx1) != "b" {
		t.Fatalf("second = %q", s.src.Slice(s.Token.Idx0, s.Token.Idx1))
	}
	if s.trivia.comments[0].Kind != ast.CommentMultiLineBlock {
		t.Fatalf("kind = %v; want MultiLineBlock", s.trivia.comments[0].Kind)
	}
}

func TestParseAnnotation(t *testing.T) {
	cases := []struct {
		src  string
		want ast.CommentContent
	}{
		{"/* @__PURE__ */", ast.ContentPure},
		{"/* #__PURE__ */", ast.ContentPure},
		{"/* @__NO_SIDE_EFFECTS__ */", ast.ContentNoSideEffects},
		{"/*! license */", ast.ContentLegal},
		{"//! x", ast.ContentLegal},
		{"/* @license */", ast.ContentLegal},
		{"/* @preserve */", ast.ContentLegal},
		{"/** jsdoc */", ast.ContentJsdoc},
		{"/** @license */", ast.ContentJsdocLegal},
		{"/* 7355685938729369933 pc=114796 dk=5 */", ast.ContentDumpMeta},
		{"/* leading */", ast.ContentNone},
		{"// line", ast.ContentNone},
		{"/***/", ast.ContentNone},
		{"/*****/", ast.ContentNone},
		{"/* #__NO_SIDE_EFFECTS__ */", ast.ContentNoSideEffects},
		{"/*@__PURE__*/", ast.ContentPure},
		{"/*#__PURE__*/", ast.ContentPure},
		{"// @__PURE__", ast.ContentPure},
		{"/* @preserve foo */", ast.ContentLegal},
		{"/* @license MIT */", ast.ContentLegal},
		{"/*!copyright*/", ast.ContentLegal},
		{"/**\n * @license\n */", ast.ContentJsdocLegal},
		{"/* 1 pc=2 dk=3 */", ast.ContentDumpMeta},
		{"/* pc=1 dk=2 */", ast.ContentNone},
		{"/* 1 pc=2 */", ast.ContentNone},
	}
	for _, tc := range cases {
		got, err := collectComments(tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("%q: len=%d; want 1", tc.src, len(got))
			continue
		}
		if got[0].Content != tc.want {
			t.Errorf("%q: content = %v; want %v", tc.src, got[0].Content, tc.want)
		}
	}
}
