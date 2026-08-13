package generator

import (
	"sort"

	"github.com/t14raptor/go-fast/ast"
)

func (g *GenVisitor) buildCommentState(cs []ast.Comment) {
	if g.opts.Minified {
		kept := make([]ast.Comment, 0, len(cs))
		for i := range cs {
			if cs[i].StayLeading() {
				kept = append(kept, cs[i])
			}
		}
		cs = kept
	}
	if len(cs) == 0 {
		return
	}
	g.comments = cs
	g.printed = make([]bool, len(cs))
	g.byAttach = make(map[ast.Idx][]int, len(cs))
	for i := range cs {
		g.byAttach[cs[i].AttachedTo] = append(g.byAttach[cs[i].AttachedTo], i)
	}
}

func (g *GenVisitor) printLeading(start ast.Idx) {
	g.printAttached(start, ast.CommentLeading)
}

func (g *GenVisitor) printTrailing(start ast.Idx) {
	g.printAttached(start, ast.CommentTrailing)
}

func (g *GenVisitor) printAttached(start ast.Idx, pos ast.CommentPosition) {
	if g.comments == nil {
		return
	}
	for _, i := range g.byAttach[start] {
		if g.printed[i] || g.comments[i].Position != pos {
			continue
		}
		g.printed[i] = true
		g.printComment(g.comments[i])
	}
}

func (g *GenVisitor) printGap(lo, hi ast.Idx) {
	if g.comments == nil || lo >= hi {
		return
	}
	cs := g.comments
	i := sort.Search(len(cs), func(i int) bool { return cs[i].Start >= lo })
	for ; i < len(cs) && cs[i].Start < hi; i++ {
		if g.printed[i] {
			continue
		}
		g.printed[i] = true
		g.printComment(cs[i])
	}
}

func (g *GenVisitor) printLegalOrphans() {
	for i := range g.comments {
		if g.printed[i] || !g.comments[i].IsLegal() {
			continue
		}
		g.printed[i] = true
		g.printComment(g.comments[i])
	}
}

func (g *GenVisitor) printAfter(prevEnd ast.Idx) {
	g.printGap(prevEnd, ast.Idx(nextTokenStart(g.src, int(prevEnd))))
}

func (g *GenVisitor) printComment(c ast.Comment) {
	if c.IsLine() {
		if g.opts.Minified {
			switch c.Content {
			case ast.ContentPure:
				g.writeString("/* @__PURE__ */ ")
			case ast.ContentNoSideEffects:
				g.writeString("/* @__NO_SIDE_EFFECTS__ */ ")
			case ast.ContentLegal, ast.ContentJsdocLegal, ast.ContentDumpMeta:
				g.writeString(c.Text(g.src))
				g.writeByte('\n')
			}
			return
		}
		g.spaceBeforeComment()
		g.writeString(c.Text(g.src))
		g.writeByte('\n')
		g.pad()
		return
	}

	atLineStart := g.atColumnZero()
	if !g.opts.Minified {
		g.spaceBeforeComment()
	}
	g.writeString(c.Text(g.src))
	if g.opts.Minified {
		return
	}
	if c.FollowedByNewline() {
		g.writeByte('\n')
		g.pad()
		return
	}
	if c.PrecededByNewline() && atLineStart {
		g.writeByte('\n')
		return
	}
	g.writeByte(' ')
}

func (g *GenVisitor) spaceBeforeComment() {
	n := len(g.buf)
	if n == 0 {
		return
	}
	switch g.buf[n-1] {
	case ' ', '\t', '\n':
		return
	}
	g.writeByte(' ')
}

func (g *GenVisitor) atColumnZero() bool {
	n := len(g.buf)
	return n == 0 || g.buf[n-1] == '\n'
}

func (g *GenVisitor) pad() {
	for range g.indent {
		g.writeByte('\t')
	}
}

// propertyStart is the first token of an object property (keyword or `...`, not the key).
func propertyStart(src string, prevEnd ast.Idx, p ast.Property) ast.Idx {
	if p.Kind() == ast.PropSpread {
		return ast.Idx(nextTokenStart(src, int(prevEnd)))
	}
	var kw ast.Idx
	switch p.Kind() {
	case ast.PropMethod:
		kw = p.MustMethod().Body.Function
	case ast.PropGetter:
		kw = p.MustGetter().Body.Function
	case ast.PropSetter:
		kw = p.MustSetter().Body.Function
	}
	if kw != 0 && kw < p.Idx0() {
		return kw
	}
	return p.Idx0()
}

func nextTokenStart(src string, i int) int {
	i = skipWSAndComments(src, i)
	if i < len(src) && src[i] == ',' {
		i++
		i = skipWSAndComments(src, i)
	}
	return i
}

func classHasName(n *ast.ClassLiteral) bool {
	return n.Name != nil && n.Name.Name != ""
}

func classHeaderHi(src string, n *ast.ClassLiteral) ast.Idx {
	if classHasName(n) {
		return n.Name.Idx
	}
	if n.SuperClass != nil {
		return n.SuperClass.Idx0()
	}
	if src == "" {
		return n.RightBrace
	}
	i := int(n.Class)
	if i+5 <= len(src) && src[i:i+5] == "class" {
		i += 5
	}
	return ast.Idx(skipWSAndComments(src, i))
}

func nextStmtStart(body []ast.Statement, i int, eof ast.Idx) ast.Idx {
	if i+1 < len(body) {
		return body[i+1].Idx0()
	}
	return eof
}

func asyncFunctionKeywordStart(src string, start ast.Idx) ast.Idx {
	i := int(start)
	if i+5 <= len(src) && src[i:i+5] == "async" {
		i += 5
	}
	return ast.Idx(skipWSAndComments(src, i))
}

func skipWSAndComments(src string, i int) int {
	for i < len(src) {
		switch src[i] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			i++
			continue
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				i += 2
				for i < len(src) && src[i] != '\n' && src[i] != '\r' {
					i++
				}
				continue
			}
			if i+1 < len(src) && src[i+1] == '*' {
				i += 2
				for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
					i++
				}
				if i+1 < len(src) {
					i += 2
				}
				continue
			}
		}
		return i
	}
	return i
}
