package generator

import (
	"sort"
	"sync"

	"github.com/t14raptor/go-fast/ast"
)

var (
	attachPool   = sync.Pool{New: func() any { return []int{} }}
	printedPool  = sync.Pool{New: func() any { return []bool{} }}
	byAttachPool = sync.Pool{New: func() any { return make(map[ast.Idx][2]int) }}
)

func getAttach(n int) []int {
	s, _ := attachPool.Get().([]int)
	if cap(s) < n {
		return make([]int, n)
	}
	return s[:n]
}

func getPrinted(n int) []bool {
	s, _ := printedPool.Get().([]bool)
	if cap(s) < n {
		return make([]bool, n)
	}
	s = s[:n]
	clear(s)
	return s
}

type commentState struct {
	src         string
	comments    []ast.Comment
	printed     []bool
	attach      []int
	byAttach    map[ast.Idx][2]int
	gapI        int
	binaryStack []commentBinaryEntry
}

type commentBinaryEntry struct {
	op        string
	rightPrec ast.Precedence
	right     *ast.Expression
	wrap      bool
	ctx       context
	leftEnd   ast.Idx
}

func (g *GenVisitor) releaseCommentState() {
	cs := g.cs
	if cs == nil {
		return
	}
	if cs.attach != nil {
		attachPool.Put(cs.attach[:0])
	}
	if cs.printed != nil {
		printedPool.Put(cs.printed[:0])
	}
	if cs.byAttach != nil {
		clear(cs.byAttach)
		byAttachPool.Put(cs.byAttach)
	}
	g.cs = nil
}

func (g *GenVisitor) buildCommentState(src string, comments []ast.Comment) {
	n := len(comments)
	if n == 0 {
		return
	}
	printed := getPrinted(n)
	if g.opts.Minified {
		keep := false
		for i := range comments {
			if comments[i].StayLeading() {
				keep = true
			} else {
				printed[i] = true
			}
		}
		if !keep {
			printedPool.Put(printed[:0])
			return
		}
	}
	attach := getAttach(n)
	for i := range comments {
		attach[i] = i
	}
	sort.Slice(attach, func(i, j int) bool {
		a, b := &comments[attach[i]], &comments[attach[j]]
		if a.AttachedTo != b.AttachedTo {
			return a.AttachedTo < b.AttachedTo
		}
		return a.Start < b.Start
	})
	byAttach := byAttachPool.Get().(map[ast.Idx][2]int)
	for i := 0; i < n; {
		at := comments[attach[i]].AttachedTo
		j := i + 1
		for j < n && comments[attach[j]].AttachedTo == at {
			j++
		}
		byAttach[at] = [2]int{i, j}
		i = j
	}
	g.cs = &commentState{
		src:      src,
		comments: comments,
		printed:  printed,
		attach:   attach,
		byAttach: byAttach,
	}
}

func (g *GenVisitor) printLeading(start ast.Idx) {
	if g.cs == nil {
		return
	}
	g.printAttached(start, ast.CommentLeading)
}

func (g *GenVisitor) printTrailing(start ast.Idx) {
	if g.cs == nil {
		return
	}
	g.printAttached(start, ast.CommentTrailing)
}

//go:noinline
func (g *GenVisitor) printAttached(start ast.Idx, pos ast.CommentPosition) {
	cs := g.cs
	r, ok := cs.byAttach[start]
	if !ok {
		return
	}
	comments := cs.comments
	idx := cs.attach
	for i := r[0]; i < r[1]; i++ {
		k := idx[i]
		if cs.printed[k] || comments[k].Position != pos {
			continue
		}
		cs.printed[k] = true
		g.printComment(comments[k])
	}
}

func (g *GenVisitor) printGap(lo, hi ast.Idx) {
	if g.cs == nil || lo >= hi {
		return
	}
	g.printGapBody(lo, hi)
}

//go:noinline
func (g *GenVisitor) printGapBody(lo, hi ast.Idx) {
	cs := g.cs
	comments := cs.comments
	i := cs.gapI
	if i >= len(comments) || comments[i].Start < lo || i > 0 && comments[i-1].Start >= lo {
		i = sort.Search(len(comments), func(i int) bool { return comments[i].Start >= lo })
	}
	for ; i < len(comments) && comments[i].Start < hi; i++ {
		if cs.printed[i] {
			continue
		}
		cs.printed[i] = true
		g.printComment(comments[i])
	}
	cs.gapI = i
}

func (g *GenVisitor) printLegalOrphans() {
	cs := g.cs
	for i := range cs.comments {
		if cs.printed[i] || !cs.comments[i].IsLegal() {
			continue
		}
		cs.printed[i] = true
		g.printComment(cs.comments[i])
	}
}

func (g *GenVisitor) printAfter(prevEnd ast.Idx) {
	if g.cs == nil {
		return
	}
	g.printGap(prevEnd, ast.Idx(nextTokenStart(g.cs.src, int(prevEnd))))
}

func (g *GenVisitor) printComment(c ast.Comment) {
	text := c.Text(g.cs.src)
	if c.IsLine() {
		if g.opts.Minified {
			switch c.Content {
			case ast.ContentPure:
				g.writeString("/* @__PURE__ */ ")
			case ast.ContentNoSideEffects:
				g.writeString("/* @__NO_SIDE_EFFECTS__ */ ")
			case ast.ContentLegal, ast.ContentJsdocLegal, ast.ContentDumpMeta:
				g.writeString(text)
				g.buf = append(g.buf, '\n')
			}
			return
		}
		g.spaceBeforeComment()
		g.buf = append(g.buf, text...)
		g.buf = append(g.buf, '\n')
		g.pad()
		return
	}

	if g.opts.Minified {
		g.writeString(text)
		return
	}

	atLineStart := g.atColumnZero()
	g.spaceBeforeComment()
	g.buf = append(g.buf, text...)
	if c.FollowedByNewline() {
		g.buf = append(g.buf, '\n')
		g.pad()
		return
	}
	if c.PrecededByNewline() && atLineStart {
		g.buf = append(g.buf, '\n')
		return
	}
	g.buf = append(g.buf, ' ')
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
	g.buf = append(g.buf, ' ')
}

func (g *GenVisitor) atColumnZero() bool {
	n := len(g.buf)
	return n == 0 || g.buf[n-1] == '\n'
}

func (g *GenVisitor) pad() {
	for range g.indent {
		g.buf = append(g.buf, '\t')
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

func classHeaderHi(n *ast.ClassLiteral) ast.Idx {
	if classHasName(n) {
		return n.Name.Idx
	}
	if n.SuperClass != nil {
		return n.SuperClass.Idx0()
	}
	if n.LeftBrace != 0 {
		return n.LeftBrace
	}
	return n.RightBrace
}

func nextStmtStart(body []ast.Statement, i int, eof ast.Idx) ast.Idx {
	if i+1 < len(body) {
		return body[i+1].Idx0()
	}
	return eof
}

// keywordIdx is the start of kw after lo, skipping whitespace and comments.
// It returns 0 when src is empty or kw is not the next token.
func keywordIdx(src string, lo ast.Idx, kw string) ast.Idx {
	if src == "" || kw == "" {
		return 0
	}
	i := skipWSAndComments(src, int(lo))
	if i+len(kw) > len(src) || src[i:i+len(kw)] != kw {
		return 0
	}
	if i+len(kw) < len(src) && isIdentContinue(src[i+len(kw)]) {
		return 0
	}
	return ast.Idx(i)
}

func isIdentContinue(c byte) bool {
	return c == '_' || c == '$' ||
		c >= 'a' && c <= 'z' ||
		c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9'
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
