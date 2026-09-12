package generator

import (
	"math"
	"strconv"
	"unsafe"

	"github.com/t14raptor/go-fast/ast"
)

// Options controls code generation behavior.
type Options struct {
	// Minified disables pretty printing. Normal // comments are dropped;
	// legal / @__PURE__ / dump tags stay as /* */.
	Minified bool
	// Comments prints Program.Comments. Off by default.
	Comments bool
}

// Generate renders node as JavaScript source using the default (pretty) options.
func Generate(node ast.VisitableNode) string {
	return GenerateWithOptions(node, Options{})
}

// GenerateMinified renders node without pretty printing (no newlines or
// indentation). It is equivalent to GenerateWithOptions(node, Options{Minified: true}).
func GenerateMinified(node ast.VisitableNode) string {
	return GenerateWithOptions(node, Options{Minified: true})
}

// GenerateWithOptions renders node as JavaScript source using the supplied options.
func GenerateWithOptions(node ast.VisitableNode, opts Options) string {
	g := &GenVisitor{opts: opts}
	g.V = g
	if opts.Comments {
		if p, ok := node.(*ast.Program); ok && len(p.Comments) > 0 {
			g.buildCommentState(p.Source, p.Comments)
			if g.cs != nil {
				hint := len(p.Source) + len(p.Comments)*8
				if hint < 256 {
					hint = 256
				}
				g.buf = make([]byte, 0, hint)
			}
		}
	}
	g.gen(node)
	if g.cs != nil {
		g.printLegalOrphans()
		g.releaseCommentState()
	}
	return unsafe.String(unsafe.SliceData(g.buf), len(g.buf))
}

type GenVisitor struct {
	ast.NoopVisitor

	buf  []byte
	opts Options

	indent int

	// Precedence/context state threaded through expression generation.
	// Set by genExpr before calling VisitWith; read by each expression visitor.
	prec ast.Precedence
	ctx  context

	binaryStack []binaryExprEntry

	cs *commentState
}

// mergeable reports whether emitting next immediately after prev would form a
// different token than intended: `++`/`+++` (a+ +b / a+ ++b), `--`, or a `//` or
// `/*` comment after a division/regex. A separating space is inserted in those
// cases. Over-spacing the harmless `a++ +b` case is acceptable; under-spacing is
// a correctness bug.
func mergeable(prev, next byte) bool {
	switch prev {
	case '+':
		return next == '+'
	case '-':
		return next == '-'
	case '/':
		return next == '/' || next == '*'
	}
	return false
}

func (g *GenVisitor) writeByte(c byte) {
	if n := len(g.buf); n > 0 && mergeable(g.buf[n-1], c) {
		g.buf = append(g.buf, ' ', c)
		return
	}
	g.buf = append(g.buf, c)
}

func (g *GenVisitor) writeString(s string) {
	if s == "" {
		return
	}
	if n := len(g.buf); n > 0 && mergeable(g.buf[n-1], s[0]) {
		g.buf = append(g.buf, ' ')
	}
	g.buf = append(g.buf, s...)
}

func (g *GenVisitor) gen(node ast.VisitableNode) {
	if g.cs != nil {
		g.genC(node)
		return
	}
	node.VisitWith(g)
}

// genExpr sets the minimum precedence and context, then visits expr. Each
// expression visitor reads g.prec/g.ctx to decide whether to wrap in parens,
// and calls genExpr on children with the appropriate child precedence.
func (g *GenVisitor) genExpr(expr *ast.Expression, prec ast.Precedence, ctx context) {
	if g.cs != nil {
		g.genExprC(expr, prec, ctx)
		return
	}
	savedPrec, savedCtx := g.prec, g.ctx
	g.prec, g.ctx = prec, ctx
	switch expr.Kind() {
	case ast.ExprBinary, ast.ExprLogical:
		g.genBinaryExpr(expr, g.prec, g.ctx)
	default:
		expr.VisitChildrenWith(g)
	}
	g.prec, g.ctx = savedPrec, savedCtx
}

func (g *GenVisitor) line() {
	if g.opts.Minified {
		return
	}
	g.writeByte('\n')
}

func (g *GenVisitor) lineAndPad() {
	if g.opts.Minified {
		return
	}
	g.writeByte('\n')
	for range g.indent {
		g.buf = append(g.buf, '\t')
	}
}

func (g *GenVisitor) space() {
	if g.opts.Minified {
		return
	}
	g.writeByte(' ')
}

func (g *GenVisitor) VisitAssignExpression(n *ast.AssignExpression) {
	if g.cs != nil {
		g.visitAssignComments(n)
		return
	}
	ctx := g.ctx
	wrap := g.prec > ast.PrecedenceAssign
	if wrap {
		g.writeByte('(')
		ctx &^= ctxForbidIn
	}

	g.gen(n.Left)
	g.space()
	g.writeString(n.Operator.String())
	g.space()
	g.genExpr(n.Right, ast.PrecedenceAssign, ctx&ctxForbidIn)

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitConditionalExpression(n *ast.ConditionalExpression) {
	if g.cs != nil {
		g.visitConditionalComments(n)
		return
	}
	ctx := g.ctx
	wrap := g.prec > ast.PrecedenceConditional
	if wrap {
		g.writeByte('(')
		ctx &^= ctxForbidIn
	}

	g.genExpr(n.Test, ast.PrecedenceConditional+1, ctx&ctxForbidIn)
	g.space()
	g.writeByte('?')
	g.space()
	g.genExpr(n.Consequent, ast.PrecedenceAssign, 0)
	g.space()
	g.writeByte(':')
	g.space()
	g.genExpr(n.Alternate, ast.PrecedenceAssign, ctx&ctxForbidIn)

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitUnaryExpression(n *ast.UnaryExpression) {
	if g.cs != nil {
		g.visitUnaryComments(n)
		return
	}
	wrap := g.prec > ast.PrecedencePrefix
	if wrap {
		g.writeByte('(')
	}

	g.writeString(n.Operator.String())
	if n.Operator.IsKeyword() {
		g.writeByte(' ')
	}
	g.genExpr(n.Operand, ast.PrecedencePrefix, 0)

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitUpdateExpression(n *ast.UpdateExpression) {
	if g.cs != nil && !n.Postfix {
		wrap := g.prec > ast.PrecedencePrefix
		if wrap {
			g.writeByte('(')
		}
		g.writeString(n.Operator.String())
		g.printGap(n.Idx, n.Operand.Idx0())
		g.genExpr(n.Operand, ast.PrecedencePrefix, 0)
		if wrap {
			g.writeByte(')')
		}
		return
	}
	if n.Postfix {
		wrap := g.prec > ast.PrecedencePostfix
		if wrap {
			g.writeByte('(')
		}

		g.genExpr(n.Operand, ast.PrecedencePostfix, 0)
		g.writeString(n.Operator.String())

		if wrap {
			g.writeByte(')')
		}
	} else {
		wrap := g.prec > ast.PrecedencePrefix
		if wrap {
			g.writeByte('(')
		}

		g.writeString(n.Operator.String())
		g.genExpr(n.Operand, ast.PrecedencePrefix, 0)

		if wrap {
			g.writeByte(')')
		}
	}
}

func (g *GenVisitor) VisitSequenceExpression(n *ast.SequenceExpression) {
	if g.cs != nil {
		ctx := g.ctx
		wrap := g.prec > ast.PrecedenceComma
		if wrap {
			g.writeByte('(')
			ctx &^= ctxForbidIn
		}
		for i := range n.Sequence {
			g.genExpr(&n.Sequence[i], ast.PrecedenceAssign, ctx&ctxForbidIn)
			if i < len(n.Sequence)-1 {
				g.writeByte(',')
				g.space()
				g.printGap(n.Sequence[i].Idx1(), n.Sequence[i+1].Idx0())
			}
		}
		if wrap {
			g.writeByte(')')
		}
		return
	}
	ctx := g.ctx
	wrap := g.prec > ast.PrecedenceComma
	if wrap {
		g.writeByte('(')
		ctx &^= ctxForbidIn
	}

	for i := range n.Sequence {
		g.genExpr(&n.Sequence[i], ast.PrecedenceAssign, ctx&ctxForbidIn)
		if i < len(n.Sequence)-1 {
			g.writeByte(',')
			g.space()
		}
	}

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitYieldExpression(n *ast.YieldExpression) {
	if g.cs != nil {
		g.visitYieldComments(n)
		return
	}
	wrap := g.prec > ast.PrecedenceYield
	if wrap {
		g.writeByte('(')
	}

	g.writeString("yield")
	if n.Delegate {
		g.writeByte('*')
	}
	if n.Argument != nil {
		g.writeByte(' ')
		g.genExpr(n.Argument, ast.PrecedenceAssign, 0)
	}

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitAwaitExpression(n *ast.AwaitExpression) {
	if g.cs != nil {
		g.visitAwaitComments(n)
		return
	}
	wrap := g.prec > ast.PrecedencePrefix
	if wrap {
		g.writeByte('(')
	}

	g.writeString("await ")
	g.genExpr(n.Argument, ast.PrecedencePrefix, 0)

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitSpreadElement(n *ast.SpreadElement) {
	g.writeString("...")
	g.genExpr(n.Expression, ast.PrecedenceAssign, 0)
}

func (g *GenVisitor) VisitCallExpression(n *ast.CallExpression) {
	if g.cs != nil {
		g.visitCallComments(n)
		return
	}
	wrap := g.ctx&ctxForbidCall != 0
	if wrap {
		g.writeByte('(')
	}

	callee, optional := optionalBase(n.Callee)
	g.genAccessHead(callee, ast.PrecedenceCall, false)
	if optional {
		g.writeString("?.")
	}
	g.writeByte('(')
	for i := range n.ArgumentList {
		g.genExpr(&n.ArgumentList[i], ast.PrecedenceAssign, 0)
		if i < len(n.ArgumentList)-1 {
			g.writeByte(',')
			g.space()
		}
	}
	g.writeByte(')')

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitNewExpression(n *ast.NewExpression) {
	if g.cs != nil {
		g.visitNewComments(n)
		return
	}
	g.writeString("new ")
	g.genExpr(n.Callee, ast.PrecedenceNew, ctxForbidCall)
	g.writeByte('(')
	for i := range n.ArgumentList {
		g.genExpr(&n.ArgumentList[i], ast.PrecedenceAssign, 0)
		if i < len(n.ArgumentList)-1 {
			g.writeByte(',')
			g.space()
		}
	}
	g.writeByte(')')
}

func (g *GenVisitor) VisitMemberExpression(n *ast.MemberExpression) {
	object, optional := optionalBase(n.Object)
	g.genAccessHead(object, ast.PrecedenceMember, true)
	if g.cs != nil {
		g.genMemberProperty(n.Property, optional, n.Object.Idx1())
		return
	}
	g.genMemberProperty(n.Property, optional, 0)
}

func (g *GenVisitor) VisitPrivateDotExpression(n *ast.PrivateDotExpression) {
	left, optional := optionalBase(n.Left)
	g.genAccessHead(left, ast.PrecedenceMember, true)
	if optional {
		g.writeByte('?')
	}
	g.writeString(".#")
	g.writeString(n.Identifier.Identifier.Name)
}

// optionalBase unwraps an Optional operand — the object/callee sitting just
// before an optional `?.` access — reporting whether one was present.
func optionalBase(expr *ast.Expression) (*ast.Expression, bool) {
	if o, ok := expr.Optional(); ok {
		return o.Expr, true
	}
	return expr, false
}

// genAccessHead emits the object/callee of a member or call, parenthesizing it
// when a bare emission would misparse or change meaning: an object/function/
// class literal (block or declaration at statement start), a nested optional
// chain (its short-circuit must stay bounded, so (a?.b).c != a?.b.c), or — for
// member access (wrapNumber) — a numeric literal (the dot in `5 .x`).
func (g *GenVisitor) genAccessHead(expr *ast.Expression, prec ast.Precedence, wrapNumber bool) {
	wrap := false
	switch expr.Kind() {
	case ast.ExprObjectLit, ast.ExprFuncLit, ast.ExprClassLit, ast.ExprOptionalChain:
		wrap = true
	case ast.ExprNumberLit:
		wrap = wrapNumber
	}
	if wrap {
		g.writeByte('(')
		g.genExpr(expr, ast.PrecedenceLowest, 0)
		g.writeByte(')')
		return
	}
	g.genExpr(expr, prec, 0)
}

func (g *GenVisitor) VisitOptionalChain(n *ast.OptionalChain) {
	g.genExpr(n.Base, ast.PrecedenceCall, 0)
}

func (g *GenVisitor) VisitOptional(n *ast.Optional) {
	g.genExpr(n.Expr, ast.PrecedenceCall, 0)
}

func (g *GenVisitor) VisitArrowFunctionLiteral(n *ast.ArrowFunctionLiteral) {
	if g.cs != nil {
		g.visitArrowComments(n)
		return
	}
	wrap := g.prec > ast.PrecedenceAssign
	if wrap {
		g.writeByte('(')
	}

	if n.Async {
		g.writeString("async ")
	}
	g.gen(n.ParameterList)
	g.space()
	g.writeString("=>")
	g.space()
	switch n.Body.Kind() {
	case ast.ConciseBodyBlock:
		g.gen(n.Body)
	case ast.ConciseBodyExpr:
		body := n.Body.MustExpr()
		if body.IsObjectLit() {
			g.writeByte('(')
			g.genExpr(body, ast.PrecedenceLowest, 0)
			g.writeByte(')')
		} else {
			g.genExpr(body, ast.PrecedenceAssign, 0)
		}
	}

	if wrap {
		g.writeByte(')')
	}
}

func (g *GenVisitor) VisitFunctionLiteral(n *ast.FunctionLiteral) {
	if n.Async {
		g.writeString("async ")
		if g.cs != nil && n.FunctionKw != 0 {
			g.printGap(n.Function, n.FunctionKw)
		}
	}
	g.writeString("function")
	if n.Generator {
		g.writeByte('*')
	}
	if n.Name != nil {
		g.writeByte(' ')
		g.gen(n.Name)
	}
	g.gen(n.ParameterList)
	g.space()
	g.gen(n.Body)
}

func (g *GenVisitor) VisitClassLiteral(n *ast.ClassLiteral) {
	if g.cs != nil {
		g.visitClassComments(n)
		return
	}
	g.writeString("class")
	if classHasName(n) {
		g.writeByte(' ')
		g.gen(n.Name)
	} else if n.Name != nil {
		// Anonymous class still has Identifier{Idx:0}; keep minify `class {}` space.
		g.writeByte(' ')
	}
	if n.SuperClass != nil {
		g.writeString(" extends ")
		g.genExpr(n.SuperClass, ast.PrecedenceAssign, 0)
	}
	g.space()
	g.writeByte('{')

	g.indent++
	for _, element := range n.Body {
		g.lineAndPad()
		switch element.Kind() {
		case ast.ClassElemMethodDef:
			e := element.MustMethodDef()
			if e.Static {
				g.writeString("static ")
			}
			switch e.Kind {
			case ast.MethodKindGet:
				g.writeString("get ")
			case ast.MethodKindSet:
				g.writeString("set ")
			default:
				if e.Body.Async {
					g.writeString("async")
					if !e.Body.Generator {
						g.writeByte(' ')
					}
				}
				if e.Body.Generator {
					g.writeByte('*')
				}
			}
			g.genPropertyName(e.Key)
			g.genMethodBody(e.Body)
		case ast.ClassElemFieldDef:
			e := element.MustFieldDef()
			if e.Static {
				g.writeString("static ")
			}
			g.genPropertyName(e.Key)
			if e.Initializer != nil {
				g.space()
				g.writeByte('=')
				g.space()
				g.genExpr(e.Initializer, ast.PrecedenceAssign, 0)
			}
			g.writeByte(';')
		case ast.ClassElemStaticBlock:
			e := element.MustStaticBlock()
			g.writeString("static")
			g.space()
			g.gen(e.Block)
		}
	}
	g.indent--

	g.lineAndPad()
	g.writeByte('}')
}

func (g *GenVisitor) VisitIdentifier(n *ast.Identifier) {
	if n != nil {
		g.writeString(n.Name)
	}
}

func (g *GenVisitor) VisitPrivateIdentifier(n *ast.PrivateIdentifier) {
	g.writeByte('#')
	g.writeString(n.Identifier.Name)
}

func (g *GenVisitor) VisitThisExpression(n *ast.ThisExpression) {
	g.writeString("this")
}

func (g *GenVisitor) VisitSuperExpression(n *ast.SuperExpression) {
	g.writeString("super")
}

func (g *GenVisitor) VisitNullLiteral(n *ast.NullLiteral) {
	g.writeString("null")
}

func (g *GenVisitor) VisitBooleanLiteral(n *ast.BooleanLiteral) {
	if n.Value {
		g.writeString("true")
	} else {
		g.writeString("false")
	}
}

func (g *GenVisitor) VisitNumberLiteral(n *ast.NumberLiteral) {
	if n.Raw != nil {
		g.writeString(*n.Raw)
	} else if math.IsInf(n.Value, 1) {
		g.writeString("Infinity")
	} else if math.IsInf(n.Value, -1) {
		wrap := g.prec > ast.PrecedencePrefix
		if wrap {
			g.writeByte('(')
		}
		g.writeString("-Infinity")
		if wrap {
			g.writeByte(')')
		}
	} else {
		g.writeString(strconv.FormatFloat(n.Value, 'f', -1, 64))
	}
}

func (g *GenVisitor) VisitBigIntLiteral(n *ast.BigIntLiteral) {
	if n.Raw != nil {
		g.writeString(*n.Raw)
		return
	}
	if n.Value != nil {
		g.writeString(n.Value.String())
	} else {
		g.writeByte('0')
	}
	g.writeByte('n')
}

func (g *GenVisitor) VisitStringLiteral(n *ast.StringLiteral) {
	if n.Raw != nil {
		g.writeString(*n.Raw)
	} else {
		g.writeString(strconv.Quote(n.Value))
	}
}

func (g *GenVisitor) VisitRegExpLiteral(n *ast.RegExpLiteral) {
	g.writeString(n.Literal)
}

func (g *GenVisitor) VisitTemplateLiteral(n *ast.TemplateLiteral) {
	if n.Tag != nil {
		g.genAccessHead(n.Tag, ast.PrecedenceCall, false)
	}
	g.writeByte('`')
	for i, e := range n.Elements {
		g.writeString(e.Literal)
		if i < len(n.Expressions) {
			g.writeString("${")
			g.genExpr(&n.Expressions[i], ast.PrecedenceLowest, 0)
			g.writeByte('}')
		}
	}
	g.writeByte('`')
}

func (g *GenVisitor) VisitArrayLiteral(n *ast.ArrayLiteral) {
	g.writeByte('[')
	if g.cs != nil {
		prevEnd := n.LeftBracket + 1
		for i, ex := range n.Value {
			if !ex.IsNone() {
				g.printAfter(prevEnd)
				g.genExpr(&n.Value[i], ast.PrecedenceAssign, 0)
				prevEnd = n.Value[i].Idx1()
			}
			if i < len(n.Value)-1 {
				g.writeByte(',')
				g.space()
			}
		}
		g.printGap(prevEnd, n.RightBracket)
		g.writeByte(']')
		return
	}
	for i, ex := range n.Value {
		if !ex.IsNone() {
			g.genExpr(&n.Value[i], ast.PrecedenceAssign, 0)
		}
		if i < len(n.Value)-1 {
			g.writeByte(',')
			g.space()
		}
	}
	g.writeByte(']')
}

func (g *GenVisitor) VisitObjectLiteral(n *ast.ObjectLiteral) {
	g.writeByte('{')

	g.indent++
	if g.cs != nil {
		prevEnd := n.LeftBrace + 1
		for i := range n.Value {
			g.lineAndPad()
			g.printGap(prevEnd, propertyStart(g.cs.src, prevEnd, n.Value[i]))
			n.Value[i].VisitWith(g)
			if i < len(n.Value)-1 {
				g.writeByte(',')
			}
			prevEnd = n.Value[i].Idx1()
		}
		g.indent--
		if len(n.Value) > 0 {
			g.lineAndPad()
		}
		g.printGap(prevEnd, n.RightBrace)
		g.writeByte('}')
		return
	}
	for i := range n.Value {
		g.lineAndPad()
		n.Value[i].VisitWith(g)
		if i < len(n.Value)-1 {
			g.writeByte(',')
		}
	}
	g.indent--

	if len(n.Value) > 0 {
		g.lineAndPad()
	}
	g.writeByte('}')
}

func (g *GenVisitor) VisitArrayPattern(n *ast.ArrayPattern) {
	g.writeByte('[')
	for i := range n.Elements {
		elem := &n.Elements[i]
		if !elem.IsNone() {
			g.gen(elem)
		}
		if i < len(n.Elements)-1 {
			g.writeByte(',')
			g.space()
		}
	}
	if n.Rest != nil {
		if len(n.Elements) > 0 {
			g.writeByte(',')
			g.space()
		}
		g.writeString("...")
		g.gen(n.Rest)
	}
	g.writeByte(']')
}

func (g *GenVisitor) VisitObjectPattern(n *ast.ObjectPattern) {
	g.writeByte('{')
	for i := range n.Properties {
		g.gen(&n.Properties[i])
		if i < len(n.Properties)-1 {
			g.writeByte(',')
			g.space()
		}
	}
	if n.Rest != nil {
		if len(n.Properties) > 0 {
			g.writeByte(',')
			g.space()
		}
		g.writeString("...")
		g.gen(n.Rest)
	}
	g.writeByte('}')
}

func (g *GenVisitor) VisitMetaProperty(n *ast.MetaProperty) {
	g.writeString(n.Kind.String())
}

func (g *GenVisitor) VisitPattern(n *ast.Pattern) {
	switch n.Kind() {
	case ast.PatternArrayPat:
		g.gen(n.MustArrayPat())
	case ast.PatternObjectPat:
		g.gen(n.MustObjectPat())
	case ast.PatternAssign:
		a := n.MustAssign()
		g.gen(a.Left)
		g.space()
		g.writeByte('=')
		g.space()
		g.genExpr(a.Right, ast.PrecedenceAssign, 0)
	default:
		// identifier / member / private-dot / invalid
		expr := ast.ExpressionFromPattern(n)
		g.genExpr(&expr, ast.PrecedenceLowest, 0)
	}
}

func (g *GenVisitor) VisitPatternKeyValue(n *ast.PatternKeyValue) {
	g.genPropertyName(n.Key)
	g.writeByte(':')
	g.space()
	g.gen(n.Value)
}

func (g *GenVisitor) VisitPatternShorthand(n *ast.PatternShorthand) {
	g.gen(n.Name)
	if n.Initializer != nil {
		if g.cs != nil {
			g.printAfter(n.Name.Idx1())
		}
		g.space()
		g.writeByte('=')
		g.space()
		g.genExpr(n.Initializer, ast.PrecedenceAssign, 0)
	}
}

func (g *GenVisitor) VisitProgram(n *ast.Program) {
	if g.cs == nil {
		for i := range n.Body {
			g.gen(&n.Body[i])
			g.line()
		}
		return
	}
	eof := ast.Idx(len(g.cs.src))
	if len(n.Body) == 0 {
		g.printGap(0, eof)
		return
	}
	g.printGap(0, n.Body[0].Idx0())
	for i := range n.Body {
		g.gen(&n.Body[i])
		g.printGap(n.Body[i].Idx1(), nextStmtStart(n.Body, i, eof))
		g.line()
	}
}

func (g *GenVisitor) VisitStatements(n *ast.Statements) {
	end := ast.Idx(0)
	if g.cs != nil {
		end = ast.Idx(len(g.cs.src))
	}
	g.visitStmtList(*n, end)
}

func (g *GenVisitor) visitStmtList(body ast.Statements, end ast.Idx) {
	if g.cs != nil {
		for i := range body {
			g.lineAndPad()
			g.gen(&body[i])
			g.printGap(body[i].Idx1(), nextStmtStart(body, i, end))
		}
		return
	}
	for i := range body {
		g.lineAndPad()
		g.gen(&body[i])
	}
}

func (g *GenVisitor) VisitBlockStatement(n *ast.BlockStatement) {
	g.writeByte('{')

	g.indent++
	g.visitStmtList(n.List, n.RightBrace)
	g.indent--

	if len(n.List) > 0 {
		g.lineAndPad()
	}
	if g.cs != nil {
		prevEnd := n.LeftBrace + 1
		if len(n.List) > 0 {
			prevEnd = n.List[len(n.List)-1].Idx1()
		}
		g.printGap(prevEnd, n.RightBrace)
	}
	g.writeByte('}')
}

func (g *GenVisitor) VisitExpressionStatement(n *ast.ExpressionStatement) {
	switch n.Expression.Kind() {
	case ast.ExprObjectLit, ast.ExprFuncLit, ast.ExprClassLit:
		g.writeByte('(')
		g.genExpr(n.Expression, ast.PrecedenceLowest, 0)
		g.writeByte(')')
	case ast.ExprAssign:
		switch n.Expression.MustAssign().Left.Kind() {
		case ast.PatternObjectPat, ast.PatternArrayPat:
			g.writeByte('(')
			g.genExpr(n.Expression, ast.PrecedenceLowest, 0)
			g.writeByte(')')
		default:
			g.genExpr(n.Expression, ast.PrecedenceLowest, 0)
		}
	default:
		g.genExpr(n.Expression, ast.PrecedenceLowest, 0)
	}
	g.writeByte(';')
}

func (g *GenVisitor) VisitVariableDeclaration(n *ast.VariableDeclaration) {
	g.writeString(n.Kind.String())
	g.writeByte(' ')
	for i := range n.List {
		g.gen(&n.List[i])
		if i < len(n.List)-1 {
			g.writeByte(',')
			g.space()
		}
	}
	g.writeByte(';')
}

func (g *GenVisitor) VisitVariableDeclarator(n *ast.VariableDeclarator) {
	g.gen(n.Target)
	if n.Initializer != nil {
		g.space()
		g.writeByte('=')
		g.space()
		g.genExpr(n.Initializer, ast.PrecedenceAssign, 0)
	}
}

func (g *GenVisitor) VisitReturnStatement(n *ast.ReturnStatement) {
	g.writeString("return")
	if n.Argument != nil {
		g.writeByte(' ')
		g.genExpr(n.Argument, ast.PrecedenceAssign, 0)
	}
	g.writeByte(';')
}

func (g *GenVisitor) VisitThrowStatement(n *ast.ThrowStatement) {
	g.writeString("throw ")
	g.genExpr(n.Argument, ast.PrecedenceAssign, 0)
	g.writeByte(';')
}

func (g *GenVisitor) VisitIfStatement(n *ast.IfStatement) {
	g.writeString("if")
	if g.cs != nil {
		g.printAfter(n.If + 2)
		g.space()
		g.writeByte('(')
		g.genExpr(n.Test, ast.PrecedenceLowest, 0)
		g.writeByte(')')
		g.printGap(n.Test.Idx1(), n.Consequent.Idx0())
		g.space()
		g.visitIfBody(n)
		return
	}
	g.space()
	g.writeByte('(')
	g.genExpr(n.Test, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.visitIfBody(n)
}

func (g *GenVisitor) visitIfBody(n *ast.IfStatement) {
	switch n.Consequent.Kind() {
	case ast.StmtEmpty, ast.StmtBlock:
		g.gen(n.Consequent)
	default:
		g.indent++
		g.gen(n.Consequent)
		g.indent--
		g.lineAndPad()
	}
	if n.Alternate != nil {
		g.writeString(" else ")
		switch n.Alternate.Kind() {
		case ast.StmtEmpty, ast.StmtBlock, ast.StmtIf:
			g.gen(n.Alternate)
		default:
			g.indent++
			g.gen(n.Alternate)
			g.indent--
			g.lineAndPad()
		}
	}
}

func (g *GenVisitor) VisitForStatement(n *ast.ForStatement) {
	g.writeString("for")
	if g.cs != nil {
		g.printAfter(n.For + 3)
	}
	g.space()
	g.writeByte('(')
	if n.Initializer != nil {
		g.gen(n.Initializer)
	} else {
		g.writeByte(';')
	}
	g.space()

	if n.Test != nil {
		g.genExpr(n.Test, ast.PrecedenceLowest, 0)
	}
	g.writeByte(';')
	g.space()
	if n.Update != nil {
		g.genExpr(n.Update, ast.PrecedenceLowest, 0)
	}
	g.writeByte(')')
	g.space()

	switch n.Body.Kind() {
	case ast.StmtEmpty, ast.StmtBlock:
		g.gen(n.Body)
	default:
		g.indent++
		g.gen(n.Body)
		g.indent--
		g.lineAndPad()
	}
}

func (g *GenVisitor) VisitForInit(n *ast.ForInit) {
	switch n.Kind() {
	case ast.ForInitExpr:
		g.genExpr(n.MustExpr(), ast.PrecedenceLowest, ctxForbidIn)
		g.writeByte(';')
	case ast.ForInitVarDecl:
		g.gen(n.MustVarDecl())
	}
}

func (g *GenVisitor) VisitForInStatement(n *ast.ForInStatement) {
	if g.cs != nil {
		g.visitForInComments(n)
		return
	}
	g.writeString("for")
	g.space()
	g.writeByte('(')
	g.gen(n.Into)
	g.writeString(" in ")
	g.genExpr(n.Source, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.gen(n.Body)
}

func (g *GenVisitor) VisitForOfStatement(n *ast.ForOfStatement) {
	if g.cs != nil {
		g.visitForOfComments(n)
		return
	}
	g.writeString("for")
	if n.Await {
		g.writeString(" await")
	}
	g.space()
	g.writeByte('(')
	g.gen(n.Into)
	g.writeString(" of ")
	g.genExpr(n.Source, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.gen(n.Body)
}

func (g *GenVisitor) VisitForInto(n *ast.ForInto) {
	switch n.Kind() {
	case ast.ForIntoVarDecl:
		into := n.MustVarDecl()

		g.writeString(into.Kind.String())
		g.writeByte(' ')
		g.gen(&into.List)
	case ast.ForIntoPattern:
		g.gen(n.MustPattern())
	}
}

func (g *GenVisitor) VisitDoWhileStatement(n *ast.DoWhileStatement) {
	g.writeString("do ")
	g.gen(n.Body)
	g.writeString(" while(")
	g.genExpr(n.Test, ast.PrecedenceLowest, 0)
	g.writeString(");")
}

func (g *GenVisitor) VisitWhileStatement(n *ast.WhileStatement) {
	if g.cs != nil {
		g.visitWhileComments(n)
		return
	}
	g.writeString("while")
	g.space()
	g.writeByte('(')
	g.genExpr(n.Test, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.gen(n.Body)
}

func (g *GenVisitor) VisitSwitchStatement(n *ast.SwitchStatement) {
	g.writeString("switch")
	g.space()
	g.writeByte('(')
	g.genExpr(n.Discriminant, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.writeByte('{')

	g.indent++
	if g.cs == nil {
		for i := range n.Body {
			g.lineAndPad()
			g.visitSwitchCase(&n.Body[i], 0)
		}
	} else {
		for i := range n.Body {
			g.lineAndPad()
			end := n.RightBrace
			if i+1 < len(n.Body) {
				end = n.Body[i+1].Idx0()
			}
			if end == 0 {
				end = ast.Idx(len(g.cs.src))
			}
			g.visitSwitchCase(&n.Body[i], end)
		}
	}
	g.indent--

	if len(n.Body) > 0 {
		g.lineAndPad()
	}
	g.writeByte('}')
}

func (g *GenVisitor) VisitSwitchCase(n *ast.SwitchCase) {
	end := ast.Idx(0)
	if g.cs != nil {
		end = ast.Idx(len(g.cs.src))
	}
	g.visitSwitchCase(n, end)
}

func (g *GenVisitor) visitSwitchCase(n *ast.SwitchCase, end ast.Idx) {
	if g.cs != nil {
		g.printLeading(n.Case)
	}
	if n.Test != nil {
		g.writeString("case ")
		g.genExpr(n.Test, ast.PrecedenceLowest, 0)
		g.writeByte(':')
	} else {
		g.writeString("default:")
	}
	g.indent++
	g.visitStmtList(n.Consequent, end)
	g.indent--
}

func (g *GenVisitor) VisitTryStatement(n *ast.TryStatement) {
	if g.cs != nil {
		g.visitTryComments(n)
		return
	}
	g.writeString("try")
	g.space()
	g.gen(n.Body)
	if n.Catch != nil {
		g.space()
		g.writeString("catch")
		g.space()
		if n.Catch.Parameter != nil {
			g.writeByte('(')
			g.gen(n.Catch.Parameter)
			g.writeByte(')')
			g.space()
		}
		g.gen(n.Catch.Body)
	}
	if n.Finally != nil {
		g.space()
		g.writeString("finally")
		g.space()
		g.gen(n.Finally)
	}
}

func (g *GenVisitor) VisitCatchClause(n *ast.CatchClause) {
	if n.Parameter != nil {
		g.gen(n.Parameter)
	}
	g.gen(n.Body)
}

func (g *GenVisitor) VisitBreakStatement(n *ast.BreakStatement) {
	g.writeString("break")
	if n.Label != nil {
		g.writeByte(' ')
		g.gen(n.Label)
	}
	g.writeByte(';')
}

func (g *GenVisitor) VisitContinueStatement(n *ast.ContinueStatement) {
	g.writeString("continue")
	if n.Label != nil {
		g.writeByte(' ')
		g.gen(n.Label)
	}
	g.writeByte(';')
}

func (g *GenVisitor) VisitLabelledStatement(n *ast.LabelledStatement) {
	if g.cs != nil {
		g.visitLabelledComments(n)
		return
	}
	g.gen(n.Label)
	g.writeByte(':')
	g.space()
	g.gen(n.Statement)
}

func (g *GenVisitor) VisitWithStatement(n *ast.WithStatement) {
	g.writeString("with")
	g.space()
	g.writeByte('(')
	g.genExpr(n.Object, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.gen(n.Body)
}

func (g *GenVisitor) VisitDebuggerStatement(n *ast.DebuggerStatement) {
	if g.cs == nil {
		g.writeString("debugger;")
		return
	}
	g.writeString("debugger")
	g.printAfter(n.Debugger + 8)
	g.writeByte(';')
}

func (g *GenVisitor) VisitEmptyStatement(n *ast.EmptyStatement) {
	g.writeByte(';')
}

func (g *GenVisitor) VisitFunctionDeclaration(n *ast.FunctionDeclaration) {
	g.lineAndPad()
	g.VisitFunctionLiteral(n.Function)
}

func (g *GenVisitor) VisitClassDeclaration(n *ast.ClassDeclaration) {
	g.VisitClassLiteral(n.Class)
}

func (g *GenVisitor) VisitParameterList(n *ast.ParameterList) {
	g.writeByte('(')
	if g.cs == nil {
		for i := range n.List {
			g.gen(&n.List[i])
			if i < len(n.List)-1 {
				g.writeByte(',')
				g.space()
			}
		}
		if n.Rest != nil {
			if len(n.List) > 0 {
				g.writeByte(',')
				g.space()
			}
			g.writeString("...")
			g.gen(n.Rest)
		}
		g.writeByte(')')
		return
	}
	prevEnd := n.Opening + 1
	for i := range n.List {
		g.gen(&n.List[i])
		if i < len(n.List)-1 {
			g.writeByte(',')
			g.space()
			g.printGap(n.List[i].Idx1(), n.List[i+1].Idx0())
		}
		prevEnd = n.List[i].Idx1()
	}

	if n.Rest != nil {
		if len(n.List) > 0 {
			g.writeByte(',')
			g.space()
		}
		g.writeString("...")
		g.gen(n.Rest)
		prevEnd = n.Rest.Idx1()
	}
	g.printGap(prevEnd, n.Closing)
	g.writeByte(')')
}

func (g *GenVisitor) VisitMemberProperty(n *ast.MemberProperty) {
	g.genMemberProperty(n, false, 0)
}

// genMemberProperty emits the property part of a member access. When optional
// is set (the access was reached through Optional), the connector becomes the
// optional-chaining form: `.x` -> `?.x` and `[x]` -> `?.[x]`.
func (g *GenVisitor) genMemberProperty(n *ast.MemberProperty, optional bool, objEnd ast.Idx) {
	switch n.Kind() {
	case ast.MemPropIdentifier:
		if optional {
			g.writeString("?.")
		} else {
			g.writeByte('.')
		}
		if objEnd != 0 {
			g.printGap(objEnd, n.MustIdentifier().Idx)
		}
		g.gen(n.MustIdentifier())
	case ast.MemPropComputed:
		if optional {
			g.writeString("?.")
		}
		if objEnd != 0 {
			g.printGap(objEnd, n.MustComputed().Idx0())
		}
		g.writeByte('[')
		// Assignment precedence keeps a comma sequence wrapped: a[(b,c)].
		g.genExpr(n.MustComputed().Expr, ast.PrecedenceAssign, 0)
		g.writeByte(']')
	}
}

func (g *GenVisitor) genPropertyName(key *ast.PropertyName) {
	if c, ok := key.Computed(); ok {
		g.writeByte('[')
		g.genExpr(c.Expr, ast.PrecedenceAssign, 0)
		g.writeByte(']')
		return
	}
	// Identifier/string/number/bigint/private key — emit the underlying node.
	g.gen(key.Unwrap())
}

// genMethodBody emits the (params) body of a method/getter/setter.
func (g *GenVisitor) genMethodBody(f *ast.FunctionLiteral) {
	g.gen(f.ParameterList)
	g.space()
	g.gen(f.Body)
}

func (g *GenVisitor) VisitPropertyKeyValue(n *ast.PropertyKeyValue) {
	g.genPropertyName(n.Key)
	g.writeByte(':')
	g.space()
	if g.cs != nil {
		g.printGap(n.Key.Idx1(), n.Value.Idx0())
	}
	g.genExpr(n.Value, ast.PrecedenceAssign, 0)
}

func (g *GenVisitor) VisitPropertyMethod(n *ast.PropertyMethod) {
	if g.cs != nil {
		kw, key := n.Body.Function, n.Key.Idx0()
		if kw != key {
			g.printLeading(kw)
		}
		if n.Body.Async {
			g.writeString("async")
			if !n.Body.Generator {
				g.writeByte(' ')
			}
		}
		if n.Body.Generator {
			g.writeByte('*')
		}
		if kw != key {
			g.printGap(kw, key)
		}
		g.genPropertyName(n.Key)
		g.genMethodBody(n.Body)
		return
	}
	if n.Body.Async {
		g.writeString("async")
		if !n.Body.Generator {
			g.writeByte(' ')
		}
	}
	if n.Body.Generator {
		g.writeByte('*')
	}
	g.genPropertyName(n.Key)
	g.genMethodBody(n.Body)
}

func (g *GenVisitor) VisitPropertyGetter(n *ast.PropertyGetter) {
	if g.cs != nil {
		kw, key := n.Body.Function, n.Key.Idx0()
		if kw != key {
			g.printLeading(kw)
		}
		g.writeString("get ")
		if kw != key {
			g.printGap(kw, key)
		}
		g.genPropertyName(n.Key)
		g.genMethodBody(n.Body)
		return
	}
	g.writeString("get ")
	g.genPropertyName(n.Key)
	g.genMethodBody(n.Body)
}

func (g *GenVisitor) VisitPropertySetter(n *ast.PropertySetter) {
	if g.cs != nil {
		kw, key := n.Body.Function, n.Key.Idx0()
		if kw != key {
			g.printLeading(kw)
		}
		g.writeString("set ")
		if kw != key {
			g.printGap(kw, key)
		}
		g.genPropertyName(n.Key)
		g.genMethodBody(n.Body)
		return
	}
	g.writeString("set ")
	g.genPropertyName(n.Key)
	g.genMethodBody(n.Body)
}
