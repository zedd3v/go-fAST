package parser_test

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/ast"
	"github.com/t14raptor/go-fast/generator"
	"github.com/t14raptor/go-fast/parser"
)

// Comment cases from Oxc, SWC, Babel, esbuild, and V8 dump tags.
// HTML, TypeScript, and JSX are out of scope.

type commentCorpusCase struct {
	name     string
	src      string
	texts    []string // recorded comment texts in source order
	parseErr bool
	// prettyMust / prettyMustNot are substrings of Generate output.
	// Empty prettyMust means "do not assert generate" (parse-only).
	prettyMust    []string
	prettyMustNot []string
	// minifyDrops: none of these strings may appear in GenerateMinified.
	minifyDrops []string
	// minifyMust: these strings must appear in GenerateMinified.
	minifyMust []string
}

func commentCorpus() []commentCorpusCase {
	return []commentCorpusCase{
		// ---- lexical / scanner (Oxc + ES lex) ----
		{name: "empty block", src: "/**/var x", texts: []string{"/**/"}, prettyMust: []string{"/**/", "var x"}},
		{name: "empty line", src: "//\nvar x", texts: []string{"//"}, prettyMust: []string{"var x"}, minifyDrops: []string{"//"}},
		{name: "file only comment", src: "/* only */", texts: []string{"/* only */"}},
		{name: "eof line", src: "var x//eof", texts: []string{"//eof"}, prettyMust: []string{"//eof", "var x"}},
		{name: "crlf", src: "a // c\r\nb", texts: []string{"// c"}, prettyMust: []string{"// c"}},
		{name: "block holds slashes", src: "/* // not a line */ var x", texts: []string{"/* // not a line */"}, prettyMust: []string{"/* // not a line */"}},
		{name: "multiline block", src: "a /*\nmid\n*/ b", texts: []string{"/*\nmid\n*/"}, prettyMust: []string{"/*\nmid\n*/"}},
		{name: "two adjacent blocks", src: "/* a *//* b */ var x", texts: []string{"/* a */", "/* b */"}, prettyMust: []string{"/* a */", "/* b */"}},
		{name: "line then block", src: "// a\n/* b */ var x", texts: []string{"// a", "/* b */"}, prettyMust: []string{"// a", "/* b */"}},
		{name: "hashbang not a comment", src: "#!/usr/bin/env node\nvar x", texts: nil, prettyMust: []string{"var x"}, prettyMustNot: []string{"#!"}},
		{name: "unterminated block", src: "/* oops", texts: []string{"/* oops"}, parseErr: true},
		{name: "comment only slashes in regex context", src: "x = a / /*c*/ /b/", texts: []string{"/*c*/"}, prettyMust: []string{"/*c*/"}},

		// ---- classification / attach (Oxc) ----
		{name: "leading block dump", src: "/* 7355685938729369933 pc=114796 dk=5 */ var v67 = heap[2]", texts: []string{"/* 7355685938729369933 pc=114796 dk=5 */"}, prettyMust: []string{"/* 7355685938729369933 pc=114796 dk=5 */", "var v67"}, minifyMust: []string{"/* 7355685938729369933 pc=114796 dk=5 */"}},
		{name: "leading line", src: "// line\nvar x = 1", texts: []string{"// line"}, prettyMust: []string{"// line"}, minifyDrops: []string{"// line"}},
		{name: "trailing line", src: "var x = 1 // trail", texts: []string{"// trail"}, prettyMust: []string{"// trail", ";"}, prettyMustNot: []string{"// trail;"}, minifyDrops: []string{"// trail"}},
		{name: "eq exception", src: "let y = // not\n10", texts: []string{"// not"}, prettyMust: []string{"// not"}},
		{name: "paren exception", src: "foo( // not\n1)", texts: []string{"// not"}, prettyMust: []string{"// not"}},
		{name: "colon exception", src: "v = c ? a : // not\nb", texts: []string{"// not"}, prettyMust: []string{"// not"}},
		{name: "mid expression", src: "var v67 = /* mid */ heap[2]", texts: []string{"/* mid */"}, prettyMust: []string{"= /* mid */ heap"}},
		{name: "mid name", src: "var /* name */ v67 = 1", texts: []string{"/* name */"}, prettyMust: []string{"var /* name */ v67"}},
		{name: "between statements", src: "a();\n/* mid */\nb();", texts: []string{"/* mid */"}, prettyMust: []string{"/* mid */"}},
		{name: "trailing after pending block", src: "foo() /* a */ // b", texts: []string{"/* a */", "// b"}, prettyMust: []string{"/* a */", "// b"}},

		// ---- annotations (esbuild + Oxc + SWC paren-pure) ----
		{name: "pure at", src: "/* @__PURE__ */ foo()", texts: []string{"/* @__PURE__ */"}, prettyMust: []string{"/* @__PURE__ */"}, minifyMust: []string{"/* @__PURE__ */"}},
		{name: "pure hash", src: "/*#__PURE__*/ foo()", texts: []string{"/*#__PURE__*/"}, prettyMust: []string{"/*#__PURE__*/"}, minifyMust: []string{"#__PURE__"}},
		{name: "pure line minify canonical", src: "// @__PURE__\nfoo()", texts: []string{"// @__PURE__"}, prettyMust: []string{"// @__PURE__"}, minifyMust: []string{"/* @__PURE__ */"}},
		{name: "paren pure swc", src: "/*#__PURE__*/ (console.log('s'))", texts: []string{"/*#__PURE__*/"}},
		{name: "no side effects", src: "/* @__NO_SIDE_EFFECTS__ */ function f(){}", texts: []string{"/* @__NO_SIDE_EFFECTS__ */"}, prettyMust: []string{"/* @__NO_SIDE_EFFECTS__ */"}, minifyMust: []string{"@__NO_SIDE_EFFECTS__"}},
		{name: "no side effects hash", src: "/* #__NO_SIDE_EFFECTS__ */ function f(){}", texts: []string{"/* #__NO_SIDE_EFFECTS__ */"}, prettyMust: []string{"#__NO_SIDE_EFFECTS__"}},
		{name: "legal bang", src: "/*! license */ var x = 1", texts: []string{"/*! license */"}, prettyMust: []string{"/*! license */"}, minifyMust: []string{"/*! license */"}},
		{name: "legal line bang", src: "//! license\nvar x = 1", texts: []string{"//! license"}, prettyMust: []string{"//! license"}, minifyMust: []string{"//! license"}},
		{name: "legal at license", src: "/* @license MIT */ var x = 1", texts: []string{"/* @license MIT */"}, prettyMust: []string{"@license"}, minifyMust: []string{"@license"}},
		{name: "legal at preserve", src: "/* @preserve */ var x = 1", texts: []string{"/* @preserve */"}, prettyMust: []string{"@preserve"}, minifyMust: []string{"@preserve"}},
		{name: "jsdoc", src: "/** docs */ function f(){}", texts: []string{"/** docs */"}, prettyMust: []string{"/** docs */"}, minifyDrops: []string{"/** docs */"}},
		{name: "jsdoc legal", src: "/** @license */ function f(){}", texts: []string{"/** @license */"}, prettyMust: []string{"/** @license */"}, minifyMust: []string{"@license"}},
		{name: "swc nosideeffects arrow", src: "const fnB = /*#__NO_SIDE_EFFECTS__*/ (args) => {}", texts: []string{"/*#__NO_SIDE_EFFECTS__*/"}, prettyMust: []string{"#__NO_SIDE_EFFECTS__"}},

		// ---- SWC fixtures ----
		{name: "swc call posts", src: "test(123/*post: 9*/, 456/*post: 10*/);", texts: []string{"/*post: 9*/", "/*post: 10*/"}, prettyMust: []string{"/*post: 9*/"}},
		{name: "swc while block", src: "while (1) {\n    /* test */\n}", texts: []string{"/* test */"}, prettyMust: []string{"/* test */"}},
		{name: "swc switch fallthrough", src: "switch (1) {\n    case 2:\n        3;\n    // 4\n}", texts: []string{"// 4"}, prettyMust: []string{"// 4"}, prettyMustNot: []string{"} // 4"}},
		{name: "swc refresh in effect", src: "React.useEffect(() => {\n    // @refresh reset\n});", texts: []string{"// @refresh reset"}, prettyMust: []string{"// @refresh reset"}},

		// ---- Babel comments/basic (JS subset) ----
		{name: "babel surrounding call", src: "foo(/*before*/ bar() /*after*/)", texts: []string{"/*before*/", "/*after*/"}, prettyMust: []string{"/*before*/"}},
		{name: "babel comment in condition", src: "if (/*c*/ x) y", texts: []string{"/*c*/"}, prettyMust: []string{"/*c*/"}},
		{name: "babel block trailing", src: "{ a(); /* c */ }", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "babel surrounding return", src: "function f(){ /*a*/ return /*b*/ 1 /*c*/; }", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/", "/*b*/", "/*c*/"}},
		{name: "babel surrounding throw", src: "function f(){ /*a*/ throw /*b*/ 1 /*c*/; }", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/", "/*b*/", "/*c*/"}},
		{name: "babel surrounding while", src: "/*a*/ while /*b*/ (1) /*c*/ { }", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel surrounding debugger", src: "/*a*/ debugger /*b*/;", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel call trailing comma", src: "f(1, /*c*/)", texts: []string{"/*c*/"}},
		{name: "babel array trailing comma", src: "[1, /*c*/]", texts: []string{"/*c*/"}},
		{name: "babel object trailing comma", src: "({a: 1, /*c*/})", texts: []string{"/*c*/"}, prettyMust: []string{"/*c*/"}},
		{name: "babel new trailing comma", src: "new F(1, /*c*/)", texts: []string{"/*c*/"}},
		{name: "babel function comments", src: "function /*a*/ f /*b*/ ( /*c*/ a /*d*/ ) /*e*/ { }", texts: []string{"/*a*/", "/*b*/", "/*c*/", "/*d*/", "/*e*/"}, prettyMust: []string{"/*a*/", "/*c*/"}},
		{name: "babel async function", src: "async /*a*/ function /*b*/ f(){}", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"async /*a*/ function"}},
		{name: "babel arrow", src: "(a) /*c*/ => 1", texts: []string{"/*c*/"}},
		{name: "babel async arrow", src: "async /*a*/ (x) /*b*/ => x", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel class method", src: "class C { /*a*/ m /*b*/ () {} }", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel class static gen", src: "class C { /*a*/ static /*b*/ * /*c*/ g() {} }", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel object method", src: "({ /*a*/ m /*b*/ () {} })", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel try", src: "try /*a*/ { } /*b*/ catch /*c*/ (e) /*d*/ { } /*e*/ finally /*f*/ { }", texts: []string{"/*a*/", "/*b*/", "/*c*/", "/*d*/", "/*e*/", "/*f*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel sequence", src: "(a, /*c*/ b, /*d*/ c)", texts: []string{"/*c*/", "/*d*/"}, prettyMust: []string{"/*c*/", "/*d*/"}},
		{name: "babel switch fallthrough", src: "switch (x) { case 1: // fall\ncase 2: break; }", texts: []string{"// fall"}},
		{name: "babel switch no default", src: "switch (x) { case 1: break;\n// no default\n}", texts: []string{"// no default"}, prettyMust: []string{"// no default"}},
		{name: "babel class static block", src: "class C { /*a*/ static /*b*/ { } }", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"/*a*/"}},
		{name: "babel directive", src: "/*a*/ 'use strict'; /*b*/", texts: []string{"/*a*/", "/*b*/"}, prettyMust: []string{"/*a*/"}},

		// ---- generate-sensitive sites already in the design ----
		{name: "async inner function", src: "async /* x */ function f() {}", texts: []string{"/* x */"}, prettyMust: []string{"async /* x */ function"}},
		{name: "class member leading", src: "class C { /* c */ static f() {} }", texts: []string{"/* c */"}, prettyMust: []string{"/* c */ static"}},
		{name: "object async method", src: "({ /* c */ async f() {} })", texts: []string{"/* c */"}, prettyMust: []string{"/* c */ async"}},
		{name: "object async inner key", src: "({ /* c */ async /* x */ f() {} })", texts: []string{"/* c */", "/* x */"}, prettyMust: []string{"/* c */ async /* x */ f"}},
		{name: "spread inner", src: "[... /* c */ x]", texts: []string{"/* c */"}, prettyMust: []string{"... /* c */ x"}},
		{name: "spread leading", src: "[ /* c */ ...x]", texts: []string{"/* c */"}, prettyMust: []string{"/* c */ ...x"}},
		{name: "call spread leading", src: "foo( /* c */ ...a)", texts: []string{"/* c */"}, prettyMust: []string{"/* c */ ...a"}},
		{name: "leading if dump", src: "/* 1 pc=2 dk=3 */ if (x) y", texts: []string{"/* 1 pc=2 dk=3 */"}, prettyMust: []string{"/* 1 pc=2 dk=3 */"}},
		{name: "new arg list", src: "new F(a, // c\nb)", texts: []string{"// c"}, prettyMust: []string{"// c"}},
		{name: "anonymous class header", src: "(class /* x */ { f() {} })", texts: []string{"/* x */"}, prettyMust: []string{"class /* x */"}, prettyMustNot: []string{"{ /* x */"}},
		{name: "last statement trail", src: "foo(); // trail", texts: []string{"// trail"}, prettyMust: []string{"// trail"}, prettyMustNot: []string{"// trail;"}},
		{name: "switch case trail", src: "switch (x) { case 1: foo(); // trail\n}", texts: []string{"// trail"}, prettyMust: []string{"// trail"}, prettyMustNot: []string{"} // trail"}},
		{name: "yield comment", src: "function* g(){ yield /* c */ 1 }", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "await comment", src: "async function f(){ await /* c */ 1 }", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "new comment", src: "new /* c */ F()", texts: []string{"/* c */"}, prettyMust: []string{"new /* c */ F"}},
		{name: "unary comment", src: "! /* c */ x", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "binary comment", src: "a /* c */ + b", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "assign comment", src: "a /* c */ = b", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "member comment", src: "a /* c */ .b", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "optional member", src: "a /* c */ ?.b", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "computed member", src: "a[ /* c */ b]", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "conditional comments", src: "a /*q*/ ? b /*r*/ : c", texts: []string{"/*q*/", "/*r*/"}, prettyMust: []string{"/*q*/", "/*r*/"}},
		{name: "template interp", src: "`x${ /* c */ y }`", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "for header", src: "for /*a*/ (/*b*/ var i = 0; /*c*/ i < 1; /*d*/ i++) {}", texts: []string{"/*a*/", "/*b*/", "/*c*/", "/*d*/"}, prettyMust: []string{"/*b*/", "/*c*/", "/*d*/"}},
		{name: "for of", src: "for (const /*a*/ x /*b*/ of /*c*/ y) {}", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/"}},
		{name: "labelled", src: "/*a*/ L /*b*/ : /*c*/ foo()", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/"}},
		{name: "param list", src: "function f(/*a*/ a /*b*/, /*c*/ b) {}", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/", "/*c*/"}},
		{name: "default param", src: "function f(a = /* c */ 1) {}", texts: []string{"/* c */"}, prettyMust: []string{"/* c */"}},
		{name: "object prop", src: "({ /*a*/ a /*b*/ : /*c*/ 1 })", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/", "/*c*/"}},
		{name: "destructure", src: "var { /*a*/ a /*b*/ = /*c*/ 1 } = o", texts: []string{"/*a*/", "/*b*/", "/*c*/"}, prettyMust: []string{"/*a*/", "/*c*/"}},

		// ---- ASI (must still parse) ----
		{name: "asi return line", src: "function f(){ return\n// c\nx }", texts: []string{"// c"}, prettyMust: []string{"return"}},
		{name: "asi after block nl", src: "a/*\n*/b", texts: []string{"/*\n*/"}, prettyMust: []string{"a", "b"}},

		// ---- HTML not supported (document, do not treat as comment) ----
		{name: "html open is not a comment", src: "<!-- x", texts: nil, parseErr: true},
	}
}

func TestCommentCorpusParseAndTexts(t *testing.T) {
	for _, tc := range commentCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parser.Parse(tc.src)
			if tc.parseErr {
				if err == nil {
					t.Fatalf("parse(%q): want error", tc.src)
				}
			} else if err != nil {
				t.Fatalf("parse(%q): %v", tc.src, err)
			}

			var got []string
			if p != nil {
				for _, c := range p.Comments {
					got = append(got, c.Text(p.Source))
				}
			}
			if len(got) != len(tc.texts) {
				t.Fatalf("texts = %#v; want %#v", got, tc.texts)
			}
			for i := range tc.texts {
				if got[i] != tc.texts[i] {
					t.Errorf("texts[%d] = %q; want %q", i, got[i], tc.texts[i])
				}
			}
		})
	}
}

func TestCommentCorpusGenerate(t *testing.T) {
	for _, tc := range commentCorpus() {
		if tc.parseErr || (len(tc.prettyMust) == 0 && len(tc.prettyMustNot) == 0 && len(tc.minifyDrops) == 0 && len(tc.minifyMust) == 0) {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			p, err := parser.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			pretty := generator.Generate(p)
			for _, s := range tc.prettyMust {
				if !strings.Contains(pretty, s) {
					t.Errorf("pretty missing %q\n got: %q", s, pretty)
				}
			}
			for _, s := range tc.prettyMustNot {
				if strings.Contains(pretty, s) {
					t.Errorf("pretty has forbidden %q\n got: %q", s, pretty)
				}
			}
			min := generator.GenerateMinified(p)
			for _, s := range tc.minifyDrops {
				if strings.Contains(min, s) {
					t.Errorf("minify kept %q\n got: %q", s, min)
				}
			}
			for _, s := range tc.minifyMust {
				if !strings.Contains(min, s) {
					t.Errorf("minify missing %q\n got: %q", s, min)
				}
			}
		})
	}
}

func TestCommentCorpusProgramSourceAndOrder(t *testing.T) {
	src := "/* a */ var x; // b\n/* c */ y"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != src {
		t.Fatalf("Source mismatch")
	}
	if len(p.Comments) != 3 {
		t.Fatalf("len=%d; want 3", len(p.Comments))
	}
	if p.Comments[0].Start >= p.Comments[1].Start || p.Comments[1].Start >= p.Comments[2].Start {
		t.Fatalf("comments not ordered by Start: %#v", p.Comments)
	}
	if !p.Comments[0].IsLeading() || !p.Comments[1].IsTrailing() || !p.Comments[2].IsLeading() {
		t.Fatalf("positions = %v %v %v", p.Comments[0].Position, p.Comments[1].Position, p.Comments[2].Position)
	}
}

func TestHTMLCommentNotRecordedWhenItParsesAsPunct(t *testing.T) {
	// Without a HTML-comment lexer, `a<!--b` is `<` `!` `--` or an error.
	// Either way it must not appear as a JS comment.
	p, err := parser.Parse("var a = 1; <!-- nope")
	if err == nil && p != nil {
		for _, c := range p.Comments {
			if strings.Contains(c.Text(p.Source), "<!--") {
				t.Fatalf("HTML comment recorded: %q", c.Text(p.Source))
			}
		}
	}
}

func TestCommentKindLineVsBlock(t *testing.T) {
	p, err := parser.Parse("// L\n/* S */ a /*\nM\n*/ b")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Comments) != 3 {
		t.Fatalf("len=%d", len(p.Comments))
	}
	if p.Comments[0].Kind != ast.CommentLine {
		t.Fatalf("line kind = %v", p.Comments[0].Kind)
	}
	if p.Comments[1].Kind != ast.CommentSingleLineBlock {
		t.Fatalf("single block kind = %v", p.Comments[1].Kind)
	}
	if p.Comments[2].Kind != ast.CommentMultiLineBlock {
		t.Fatalf("multi block kind = %v", p.Comments[2].Kind)
	}
}
