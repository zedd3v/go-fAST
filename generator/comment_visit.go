package generator

import "github.com/t14raptor/go-fast/ast"

//go:noinline
func (g *GenVisitor) genC(node ast.VisitableNode) {
	if n, ok := node.(ast.Node); ok {
		g.printLeading(n.Idx0())
	}
	node.VisitWith(g)
	if n, ok := node.(ast.Node); ok {
		g.printTrailing(n.Idx0())
	}
}

//go:noinline
func (g *GenVisitor) genExprC(expr *ast.Expression, prec ast.Precedence, ctx context) {
	spread := expr.Kind() == ast.ExprSpread
	if !spread {
		g.printLeading(expr.Idx0())
	}
	savedPrec, savedCtx := g.prec, g.ctx
	g.prec, g.ctx = prec, ctx
	switch expr.Kind() {
	case ast.ExprBinary, ast.ExprLogical:
		g.genBinaryExprC(expr, g.prec, g.ctx)
	default:
		expr.VisitChildrenWith(g)
	}
	g.prec, g.ctx = savedPrec, savedCtx
	if !spread {
		g.printTrailing(expr.Idx0())
	}
}

//go:noinline
func (g *GenVisitor) genExprListC(args ast.Expressions, prevEnd, close ast.Idx) {
	for i := range args {
		g.printAfter(prevEnd)
		g.genExpr(&args[i], ast.PrecedenceAssign, 0)
		if i < len(args)-1 {
			g.writeByte(',')
			g.space()
		}
		prevEnd = args[i].Idx1()
	}
	g.printGap(prevEnd, close)
}

//go:noinline
func (g *GenVisitor) genBinaryExprC(expr *ast.Expression, minPrec ast.Precedence, ctx context) {
	base := len(g.cs.binaryStack)

descend:
	for {
		var opStr string
		var opPrec, leftPrec, rightPrec ast.Precedence
		var left, right *ast.Expression
		var isIn bool

		switch expr.Kind() {
		case ast.ExprBinary:
			n := expr.MustBinary()
			opStr, opPrec = n.Operator.String(), n.Operator.Precedence()
			left, right = n.Left, n.Right
			isIn = n.Operator == ast.BinaryIn
			leftPrec, rightPrec = opPrec, opPrec+1
			if opPrec.IsRightAssociative() {
				leftPrec, rightPrec = opPrec+1, opPrec
			}
			if n.Operator == ast.BinaryExponential {
				if left.IsUnary() {
					leftPrec = ast.PrecedenceCall
				}
			}
		case ast.ExprLogical:
			n := expr.MustLogical()
			opStr, opPrec = n.Operator.String(), n.Operator.Precedence()
			left, right = n.Left, n.Right
			leftPrec, rightPrec = opPrec, opPrec+1
			if n.Operator == ast.LogicalCoalesce {
				leftPrec = ast.PrecedenceLogicalAnd + 1
				rightPrec = leftPrec
			}
		default:
			g.genExpr(expr, minPrec, ctx)
			break descend
		}

		wrap := opPrec < minPrec || (isIn && ctx&ctxForbidIn != 0)
		if wrap {
			g.writeByte('(')
			ctx = 0
		} else {
			ctx &= ctxForbidIn
		}

		g.cs.binaryStack = append(g.cs.binaryStack, commentBinaryEntry{
			op:        opStr,
			rightPrec: rightPrec,
			right:     right,
			wrap:      wrap,
			ctx:       ctx,
			leftEnd:   left.Idx1(),
		})
		expr, minPrec = left, leftPrec
	}

	for {
		length := len(g.cs.binaryStack)
		if length == 0 || length-1 < base {
			break
		}
		e := g.cs.binaryStack[length-1]
		g.cs.binaryStack = g.cs.binaryStack[:length-1]
		g.printGap(e.leftEnd, e.right.Idx0())
		if e.op == "in" || e.op == "instanceof" {
			g.writeByte(' ')
			g.writeString(e.op)
			g.writeByte(' ')
		} else {
			g.space()
			g.writeString(e.op)
			g.space()
		}
		g.genExpr(e.right, e.rightPrec, e.ctx)
		if e.wrap {
			g.writeByte(')')
		}
	}
}

//go:noinline
func (g *GenVisitor) visitUnaryComments(n *ast.UnaryExpression) {
	wrap := g.prec > ast.PrecedencePrefix
	if wrap {
		g.writeByte('(')
	}
	g.writeString(n.Operator.String())
	if n.Operator.IsKeyword() {
		g.writeByte(' ')
	}
	g.printGap(n.Idx, n.Operand.Idx0())
	g.genExpr(n.Operand, ast.PrecedencePrefix, 0)
	if wrap {
		g.writeByte(')')
	}
}

//go:noinline
func (g *GenVisitor) visitYieldComments(n *ast.YieldExpression) {
	wrap := g.prec > ast.PrecedenceYield
	if wrap {
		g.writeByte('(')
	}
	g.writeString("yield")
	hi := n.Yield + 5
	if n.Argument != nil {
		hi = n.Argument.Idx0()
	}
	g.printGap(n.Yield, hi)
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

//go:noinline
func (g *GenVisitor) visitAwaitComments(n *ast.AwaitExpression) {
	wrap := g.prec > ast.PrecedencePrefix
	if wrap {
		g.writeByte('(')
	}
	g.writeString("await ")
	g.printGap(n.Await, n.Argument.Idx0())
	g.genExpr(n.Argument, ast.PrecedencePrefix, 0)
	if wrap {
		g.writeByte(')')
	}
}

//go:noinline
func (g *GenVisitor) visitAssignComments(n *ast.AssignExpression) {
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
	g.printGap(n.Left.Idx1(), n.Right.Idx0())
	g.genExpr(n.Right, ast.PrecedenceAssign, ctx&ctxForbidIn)
	if wrap {
		g.writeByte(')')
	}
}

//go:noinline
func (g *GenVisitor) visitConditionalComments(n *ast.ConditionalExpression) {
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
	g.printGap(n.Test.Idx1(), n.Consequent.Idx0())
	g.genExpr(n.Consequent, ast.PrecedenceAssign, 0)
	g.space()
	g.writeByte(':')
	g.space()
	g.printGap(n.Consequent.Idx1(), n.Alternate.Idx0())
	g.genExpr(n.Alternate, ast.PrecedenceAssign, ctx&ctxForbidIn)
	if wrap {
		g.writeByte(')')
	}
}

//go:noinline
func (g *GenVisitor) visitCallComments(n *ast.CallExpression) {
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
	g.genExprListC(n.ArgumentList, n.LeftParenthesis+1, n.RightParenthesis)
	g.writeByte(')')
	if wrap {
		g.writeByte(')')
	}
}

//go:noinline
func (g *GenVisitor) visitNewComments(n *ast.NewExpression) {
	g.writeString("new ")
	g.printGap(n.New, n.Callee.Idx0())
	g.genExpr(n.Callee, ast.PrecedenceNew, ctxForbidCall)
	g.writeByte('(')
	g.genExprListC(n.ArgumentList, n.LeftParenthesis+1, n.RightParenthesis)
	g.writeByte(')')
}

//go:noinline
func (g *GenVisitor) visitArrowComments(n *ast.ArrowFunctionLiteral) {
	wrap := g.prec > ast.PrecedenceAssign
	if wrap {
		g.writeByte('(')
	}
	if n.Async {
		g.writeString("async ")
		g.printGap(n.Start, n.ParameterList.Opening)
	}
	g.gen(n.ParameterList)
	g.printGap(n.ParameterList.Idx1(), n.Body.Idx0())
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

//go:noinline
func (g *GenVisitor) visitClassComments(n *ast.ClassLiteral) {
	g.writeString("class")
	g.printGap(n.Class, classHeaderHi(n))
	if classHasName(n) {
		g.writeByte(' ')
		g.gen(n.Name)
	} else if n.Name != nil {
		g.writeByte(' ')
	}
	if n.SuperClass != nil {
		if classHasName(n) {
			g.printGap(n.Name.Idx1(), n.SuperClass.Idx0())
		}
		g.writeString(" extends ")
		g.genExpr(n.SuperClass, ast.PrecedenceAssign, 0)
	}
	g.space()
	g.writeByte('{')
	g.indent++
	prevEnd := n.Class
	for _, element := range n.Body {
		g.lineAndPad()
		g.printGap(prevEnd, element.Idx0())
		g.printLeading(element.Idx0())
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
			g.printGap(element.Idx0(), e.Key.Idx0())
			g.genPropertyName(e.Key)
			g.genMethodBody(e.Body)
		case ast.ClassElemFieldDef:
			e := element.MustFieldDef()
			if e.Static {
				g.writeString("static ")
			}
			g.printGap(element.Idx0(), e.Key.Idx0())
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
			g.printGap(element.Idx0(), e.Block.Idx0())
			g.gen(e.Block)
		}
		prevEnd = element.Idx1()
	}
	g.indent--
	g.printGap(prevEnd, n.RightBrace)
	g.lineAndPad()
	g.writeByte('}')
}

//go:noinline
func (g *GenVisitor) visitForInComments(n *ast.ForInStatement) {
	g.writeString("for")
	g.printAfter(n.For + 3)
	g.space()
	g.writeByte('(')
	g.gen(n.Into)
	if kw := keywordIdx(g.cs.src, n.Into.Idx1(), "in"); kw != 0 {
		g.printGap(n.Into.Idx1(), kw)
	}
	g.writeString(" in ")
	g.genExpr(n.Source, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.gen(n.Body)
}

//go:noinline
func (g *GenVisitor) visitForOfComments(n *ast.ForOfStatement) {
	g.writeString("for")
	if n.Await {
		g.writeString(" await")
	}
	g.printAfter(n.For + 3)
	g.space()
	g.writeByte('(')
	g.gen(n.Into)
	if kw := keywordIdx(g.cs.src, n.Into.Idx1(), "of"); kw != 0 {
		g.printGap(n.Into.Idx1(), kw)
	}
	g.writeString(" of ")
	g.genExpr(n.Source, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.space()
	g.gen(n.Body)
}

//go:noinline
func (g *GenVisitor) visitWhileComments(n *ast.WhileStatement) {
	g.writeString("while")
	g.printAfter(n.While + 5)
	g.space()
	g.writeByte('(')
	g.genExpr(n.Test, ast.PrecedenceLowest, 0)
	g.writeByte(')')
	g.printGap(n.Test.Idx1(), n.Body.Idx0())
	g.space()
	g.gen(n.Body)
}

//go:noinline
func (g *GenVisitor) visitLabelledComments(n *ast.LabelledStatement) {
	g.gen(n.Label)
	g.printGap(n.Label.Idx1(), n.Colon)
	g.writeByte(':')
	g.printGap(n.Colon+1, n.Statement.Idx0())
	g.space()
	g.gen(n.Statement)
}

//go:noinline
func (g *GenVisitor) visitTryComments(n *ast.TryStatement) {
	g.writeString("try")
	g.printAfter(n.Try + 3)
	g.space()
	g.gen(n.Body)
	if n.Catch != nil {
		g.printGap(n.Body.Idx1(), n.Catch.Catch)
		g.space()
		g.writeString("catch")
		g.printAfter(n.Catch.Catch + 5)
		g.space()
		if n.Catch.Parameter != nil {
			g.writeByte('(')
			g.gen(n.Catch.Parameter)
			g.writeByte(')')
			g.printGap(n.Catch.Parameter.Idx1(), n.Catch.Body.Idx0())
			g.space()
		}
		g.gen(n.Catch.Body)
	}
	if n.Finally != nil {
		prev := n.Body.Idx1()
		if n.Catch != nil {
			prev = n.Catch.Body.Idx1()
		}
		if kw := keywordIdx(g.cs.src, prev, "finally"); kw != 0 {
			g.printGap(prev, kw)
		}
		g.space()
		g.writeString("finally")
		g.space()
		g.gen(n.Finally)
	}
}
