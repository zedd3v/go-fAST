package generator

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/parser"
	"github.com/t14raptor/go-fast/resolver"
)

func assertMinified(t *testing.T, input, want string) {
	t.Helper()

	p, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse input: %v", err)
	}

	got := GenerateMinified(p)
	if got != want {
		t.Fatalf("gen(%q) = %q; want %q", input, got, want)
	}
}

func TestMinifiedOperatorTokenBoundaries(t *testing.T) {
	assertMinified(t, `a + ++b;`, `a+ ++b;`)
	assertMinified(t, `a - --b;`, `a- --b;`)
	assertMinified(t, `a + +b;`, `a+ +b;`)
	assertMinified(t, `a - -b;`, `a- -b;`)
	assertMinified(t, `x = a / /b/.source;`, `x=a/ /b/.source;`)
	assertMinified(t, `x = a / /b/();`, `x=a/ /b/();`)
}

func TestMetaProperty(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`function Foo(){new.target;}`, `function Foo(){new.target;}`},
		{`function Foo(){if(new.target){}}`, `function Foo(){if(new.target){}}`},
		{`function Foo(){let x=new.target;}`, `function Foo(){let x=new.target;}`},
	}
	for _, tt := range tests {
		p, err := parser.Parse(tt.in)
		if err != nil {
			t.Fatalf("Failed to parse input: %v", err)
		}

		got := GenerateMinified(p)
		if got != tt.want {
			t.Errorf("gen(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestMethodKindGetSetKeywords(t *testing.T) {
	assertMinified(t,
		`({get value(){return 1;},set value(next){this.next=next;}});`,
		`({get value(){return 1;},set value(next){this.next=next;}});`,
	)
}

func TestForInitializerForbidInRegressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "assignment rhs",
			input: "for (x = (a in b);;) {}",
			want:  "for(x=(a in b);;){}",
		},
		{
			name:  "sequence element",
			input: "for (x, (a in b);;) {}",
			want:  "for(x,(a in b);;){}",
		},
		{
			name:  "conditional test",
			input: "for (((a in b) ? c : d);;) {}",
			want:  "for((a in b)?c:d;;){}",
		},
		{
			name:  "conditional alternate",
			input: "for ((a ? b : (c in d));;) {}",
			want:  "for(a?b:(c in d);;){}",
		},
		{
			name:  "binary left subtree",
			input: "for (((a in b) && c);;) {}",
			want:  "for((a in b)&&c;;){}",
		},
		{
			name:  "binary right subtree",
			input: "for (a && (b in c);;) {}",
			want:  "for(a&&(b in c);;){}",
		},
		{
			name:  "wrapped conditional test clears forbid-in",
			input: "for (((a in b) ? c : d) * e;;) {}",
			want:  "for((a in b?c:d)*e;;){}",
		},
		{
			name:  "wrapped conditional alternate clears forbid-in",
			input: "for ((a ? b : (c in d)) * e;;) {}",
			want:  "for((a?b:c in d)*e;;){}",
		},
		{
			name:  "wrapped assignment clears forbid-in",
			input: "for (1 * (x = (a in b));;) {}",
			want:  "for(1*(x=a in b);;){}",
		},
		{
			name:  "nested wrapped sequence clears forbid-in",
			input: "for ((x, (a in b)) * c;;) {}",
			want:  "for((x,a in b)*c;;){}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMinified(t, tt.input, tt.want)
		})
	}
}

func TestBinaryExprNestedRightRegressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "binary right subtree",
			input: "c >> (d & e);",
			want:  "c>>(d&e);",
		},
		{
			name:  "conditional consequent binary right subtree",
			input: "a && b ? c >> (d & e) : f;",
			want:  "a&&b?c>>(d&e):f;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMinified(t, tt.input, tt.want)
		})
	}
}

func TestSequenceExpressionInNewExpression(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "sequence as single argument to new",
			input:    "new F6(((a=1),2));",
			expected: "new F6((a=1,2));",
		},
		{
			name:     "sequence as second argument to new",
			input:    "new F6(x,((b=2),3));",
			expected: "new F6(x,(b=2,3));",
		},
		{
			name:     "sequence as third argument to new",
			input:    "new F6(x,y,((c=3),4));",
			expected: "new F6(x,y,(c=3,4));",
		},
		{
			name:     "sequence with function literal in new",
			input:    "new F6(h,((r=R),function(W){return r++;}));",
			expected: "new F6(h,(r=R,function(W){return r++;}));",
		},
		{
			name:     "sequence in regular function call (should work)",
			input:    "f(((d=4),5));",
			expected: "f((d=4,5));",
		},
		{
			name:     "sequence as second argument in regular call (should work)",
			input:    "f(x,((e=5),6));",
			expected: "f(x,(e=5,6));",
		},
		{
			name:     "sequence in throw statement",
			input:    "throw ((a=1),2);",
			expected: "throw (a=1,2);",
		},
		{
			name:     "sequence in await expression",
			input:    "async function f(){await ((b=2),3);}",
			expected: "async function f(){await (b=2,3);}",
		},
		{
			name:     "sequence in return statement",
			input:    "function g(){return ((d=4),5);}",
			expected: "function g(){return (d=4,5);}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse input: %v", err)
			}

			result := GenerateMinified(ctx)
			if result != tt.expected {
				t.Errorf("\nInput:    %s\nExpected: %s\nGot:      %s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestPatternRoundTrip(t *testing.T) {
	cases := []struct{ in, want string }{
		{`var [a, , b = 1, ...c] = x;`, `var [a,,b=1,...c]=x;`},
		{`var {a, b: {c} = {}, ...r} = o;`, `var {a,b:{c}={},...r}=o;`},
		{`for (const [k, v] of m) {}`, `for(const [k,v] of m){}`},
		{`for ([x.y, z] of p) {}`, `for([x.y,z] of p){}`},
		{`try {} catch ({message}) {}`, `try{}catch({message}){}`},
		{`([x.y, z] = p);`, `([x.y,z]=p);`},
		{`function f([a] = [], {b} = {}, ...rest) {}`, `function f([a]=[],{b}={},...rest){}`},
		{`({a = 1} = o);`, `({a=1}=o);`},
		{`label: x = 1;`, `label:x=1;`},
		{`obj.x = 1;`, `obj.x=1;`},
	}
	for _, c := range cases {
		p, err := parser.Parse(c.in)
		if err != nil {
			t.Fatalf("parse(%q): %v", c.in, err)
		}
		got := GenerateMinified(p)
		if got != c.want {
			t.Errorf("gen(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestPatternEdgeRoundTrip(t *testing.T) {
	ok := []struct{ in, want string }{
		{`var [a, ...[b, c]] = x;`, `var [a,...[b,c]]=x;`},     // array rest is a nested pattern
		{`function f(...[a, b]) {}`, `function f(...[a,b]){}`}, // param rest is a pattern
		{`var {a, ...rest} = o;`, `var {a,...rest}=o;`},        // object rest ident
		{`[a, , b] = c;`, `([a,,b]=c);`},                       // assignment with elision hole
		{`({a, b} = c);`, `({a,b}=c);`},                        // parenthesised object assign
		{`for ({a} of x) {}`, `for({a} of x){}`},               // for-of object pattern target
		{`for ([a] in x) {}`, `for([a] in x){}`},               // for-in array pattern target
		{`function f({a = 1, b: {c} = {}}) {}`, `function f({a=1,b:{c}={}}){}`},
		{`var {[k]: v = 1} = o;`, `var {[k]:v=1}=o;`}, // computed key + default
		{`let [a, b = a] = x;`, `let [a,b=a]=x;`},     // sibling default reference
		{`var [a = b.c] = o;`, `var [a=b.c]=o;`},      // default value may be a member
	}
	for _, c := range ok {
		p, err := parser.Parse(c.in)
		if err != nil {
			t.Errorf("parse(%q): %v", c.in, err)
			continue
		}
		resolver.Resolve(p) // must not panic
		if got := GenerateMinified(p); got != c.want {
			t.Errorf("gen(%q) = %q; want %q", c.in, got, c.want)
		}
	}

	bad := []string{
		`var {...{a}} = o;`, // object rest must be a simple target
		`var [a.b] = c;`,    // member in binding position
		`var {a: b.c} = o;`, // member value in binding position
	}
	for _, src := range bad {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("parse(%q): expected error, got nil", src)
		}
	}
}

func TestOptionalChainingMinified(t *testing.T) {
	assertMinified(t, `a?.b; a?.(b); a?.[b];`, `a?.b;a?.(b);a?.[b];`)
	assertMinified(t, `(function(){})?.();`, `(function(){})?.();`)
	assertMinified(t, `(function(){})?.x;`, `(function(){})?.x;`)
}

func TestLiteralMemberAndCallBasesMinified(t *testing.T) {
	assertMinified(t, `(function(){}).x;`, `(function(){}).x;`)
	assertMinified(t, `(class {}).x;`, `(class {}).x;`)
	assertMinified(t, `(class {})();`, `(class {})();`)
	assertMinified(t, `({[(a,b)]:1});`, `({[(a,b)]:1});`)
}

func TestComputedMemberSequenceMinified(t *testing.T) {
	assertMinified(t, `a[(b,c)]; a?.[(b,c)];`, `a[(b,c)];a?.[(b,c)];`)
}

func TestClassExtendsAndSuperMinified(t *testing.T) {
	assertMinified(t,
		`class A extends B { constructor(){ super(); super.x; } }`,
		`class A extends B{constructor(){super();super.x;}}`,
	)
	assertMinified(t, `class A extends (a, b) {}`, `class A extends (a,b){}`)
}

func TestGeneratorFunctionsAndObjectMethodsMinified(t *testing.T) {
	assertMinified(t, `function* g(){ yield 1; }`, `function* g(){yield 1;}`)
	assertMinified(t, `const g = function*(){ yield 1; };`, `const g=function*(){yield 1;};`)
	assertMinified(t, `async function* g(){ yield 1; }`, `async function* g(){yield 1;}`)
	assertMinified(t,
		`({ m(){ return super.x; }, *g(){ yield 1; } });`,
		`({m(){return super.x;},*g(){yield 1;}});`,
	)
}

func TestTemplateLiteralMinified(t *testing.T) {
	assertMinified(t, "tag`x${y}`;", "tag`x${y}`;")
	assertMinified(t, "`\\${x}`;", "`\\${x}`;")
	assertMinified(t, "`\\n`;", "`\\n`;")
	assertMinified(t, "(function(){})`x`;", "(function(){})`x`;")
	assertMinified(t, "(class {})`x`;", "(class {})`x`;")
	assertMinified(t, "({})`x`;", "({})`x`;")
}

func TestGenerateSkipComments(t *testing.T) {
	src := "/* a */ var x = 1 // b"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Comments) == 0 {
		t.Fatal("parse dropped comments")
	}
	got := GenerateWithOptions(p, Options{SkipComments: true})
	if strings.Contains(got, "/* a */") || strings.Contains(got, "// b") {
		t.Fatalf("SkipComments printed comments: %q", got)
	}
	if !strings.Contains(got, "var x") {
		t.Fatalf("SkipComments dropped code: %q", got)
	}
}

func TestLeadingDumpMetaEmitted(t *testing.T) {
	src := "/* 7355685938729369933 pc=114796 dk=5 */ var v67 = heap[2]"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	wantPretty := "/* 7355685938729369933 pc=114796 dk=5 */\nvar v67 = heap[2];\n"
	if got := Generate(p); got != wantPretty {
		t.Errorf("Generate = %q; want %q", got, wantPretty)
	}
	wantMin := "/* 7355685938729369933 pc=114796 dk=5 */var v67=heap[2];"
	if got := GenerateMinified(p); got != wantMin {
		t.Errorf("GenerateMinified = %q; want %q", got, wantMin)
	}
	if got := GenerateMinified(p); strings.Contains(got, "//") {
		t.Errorf("GenerateMinified rewrote dump meta as //: %q", got)
	}
}

func TestLeadingDumpMetaOnExpressionStatement(t *testing.T) {
	src := "/* 7355685938729369933 pc=114796 dk=5 */ heap[2]"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	wantPretty := "/* 7355685938729369933 pc=114796 dk=5 */\nheap[2];\n"
	if got := Generate(p); got != wantPretty {
		t.Errorf("Generate = %q; want %q", got, wantPretty)
	}
	wantMin := "/* 7355685938729369933 pc=114796 dk=5 */heap[2];"
	if got := GenerateMinified(p); got != wantMin {
		t.Errorf("GenerateMinified = %q; want %q", got, wantMin)
	}
}

func TestMidExpressionBlockComment(t *testing.T) {
	src := "var v67 = /* mid */ heap[2]"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := "var v67 = /* mid */ heap[2];\n"
	if got := Generate(p); got != want {
		t.Errorf("Generate = %q; want %q", got, want)
	}
}

func TestTrailingLineCommentDoesNotSwallowSemicolon(t *testing.T) {
	src := "var x = 1 // trail"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, ";") {
		t.Fatalf("pretty missing semicolon: %q", got)
	}
	if strings.Contains(got, "// trail;") {
		t.Fatalf("line comment swallowed semicolon: %q", got)
	}
	if !strings.Contains(got, "// trail") {
		t.Fatalf("pretty dropped trailing comment: %q", got)
	}
}

func TestLastStatementTrailingLineComment(t *testing.T) {
	src := "foo(); // trail"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "// trail") {
		t.Fatalf("last-statement trailing comment missing: %q", got)
	}
	if strings.Contains(got, "// trail;") {
		t.Fatalf("line comment swallowed semicolon: %q", got)
	}
}

func TestMinifyDropsNormalLineComment(t *testing.T) {
	assertMinified(t, "// line\nvar x = 1", "var x=1;")
}

func TestMinifyPureCanonical(t *testing.T) {
	src := "// @__PURE__\nfoo()"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := GenerateMinified(p)
	want := "/* @__PURE__ */ foo();"
	if got != want {
		t.Errorf("GenerateMinified = %q; want %q", got, want)
	}
	if strings.Contains(got, "/*  @__PURE__") || strings.Contains(got, "*/ x */") {
		t.Errorf("wrapped line body into a block: %q", got)
	}
}

func TestMinifyPureIdempotent(t *testing.T) {
	src := "/* @__PURE__ */foo();"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	once := GenerateMinified(p)
	p2, err := parser.Parse(once)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	twice := GenerateMinified(p2)
	if once != twice {
		t.Errorf("minify not idempotent: %q then %q", once, twice)
	}
}

func TestAsyncInnerComment(t *testing.T) {
	src := "async /* x */ function f() {}"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "async /* x */ function") {
		t.Fatalf("async inner comment misplaced: %q", got)
	}
}

func TestClassMemberLeadingComment(t *testing.T) {
	src := "class C { /* c */ static f() {} }"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "/* c */ static") {
		t.Fatalf("class member comment misplaced: %q", got)
	}
}

func TestObjectAsyncMethodLeadingComment(t *testing.T) {
	src := "({ /* c */ async f() {} })"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "/* c */ async") {
		t.Fatalf("object method comment misplaced: %q", got)
	}
}

func TestLegalOrphanAtEOF(t *testing.T) {
	src := "/*! license */ var x = 1"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p.Body = nil
	got := Generate(p)
	if !strings.Contains(got, "/*! license */") {
		t.Fatalf("legal orphan dropped: %q", got)
	}
}

func TestObjectAsyncMethodInnerKeyComment(t *testing.T) {
	src := "({ /* c */ async /* x */ f() {} })"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "/* c */ async /* x */ f") {
		t.Fatalf("object method comments misplaced: %q", got)
	}
}

func TestSpreadInnerAndLeadingComments(t *testing.T) {
	p, err := parser.Parse("[... /* c */ x]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "... /* c */ x") {
		t.Fatalf("spread inner comment misplaced: %q", got)
	}

	p, err = parser.Parse("[ /* c */ ...x]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got = Generate(p)
	if !strings.Contains(got, "/* c */ ...x") {
		t.Fatalf("leading comment on ... dropped: %q", got)
	}

	p, err = parser.Parse("foo( /* c */ ...a)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got = Generate(p)
	if !strings.Contains(got, "/* c */ ...a") {
		t.Fatalf("call leading comment on ... dropped: %q", got)
	}
}

func TestLeadingCommentOnIf(t *testing.T) {
	src := "/* 1 pc=2 dk=3 */ if (x) y"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Body[0].Idx0() == 0 {
		t.Fatalf("If.Idx0 is 0; keyword index was not set")
	}
	got := Generate(p)
	if !strings.Contains(got, "/* 1 pc=2 dk=3 */") || !strings.Contains(got, "if") {
		t.Fatalf("leading if comment dropped: %q", got)
	}
	if strings.Index(got, "/* 1 pc=2 dk=3 */") > strings.Index(got, "if") {
		t.Fatalf("leading if comment after if: %q", got)
	}
}

func TestSwitchCaseTrailingCommentStaysInCase(t *testing.T) {
	src := "switch (x) { case 1: foo(); // trail\n}"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if strings.Contains(got, "} // trail") || strings.Contains(got, "}\n// trail") {
		t.Fatalf("case trailing comment printed after }: %q", got)
	}
	if !strings.Contains(got, "// trail") {
		t.Fatalf("case trailing comment dropped: %q", got)
	}
	if i, j := strings.Index(got, "// trail"), strings.Index(got, "}"); i < 0 || j < 0 || i > j {
		t.Fatalf("case trailing comment not before }: %q", got)
	}
}

func TestNewExpressionArgListComment(t *testing.T) {
	src := "new F(a, // c\nb)"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "// c") {
		t.Fatalf("new arg comment dropped: %q", got)
	}
}

func TestAnonymousClassHeaderComment(t *testing.T) {
	src := "(class /* x */ { f() {} })"
	p, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Generate(p)
	if !strings.Contains(got, "class /* x */") {
		t.Fatalf("anonymous class header comment misplaced: %q", got)
	}
	if strings.Contains(got, "{ /* x */") {
		t.Fatalf("anonymous class comment moved into body: %q", got)
	}
}
