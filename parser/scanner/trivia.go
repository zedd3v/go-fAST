package scanner

import (
	"strings"

	"github.com/t14raptor/go-fast/ast"
	"github.com/t14raptor/go-fast/parser/scanner/token"
)

// triviaBuilder records comments and classifies them at lex time.
// The zero value is not valid: sawNewline must start true.
type triviaBuilder struct {
	comments []ast.Comment

	processed int

	sawNewline           bool
	sawNewlineForComment bool
	previousKind         token.Token
	previousStart        ast.Idx
}

func newTriviaBuilder() triviaBuilder {
	return triviaBuilder{
		sawNewline:           true,
		sawNewlineForComment: true,
		previousKind:         token.Undetermined,
	}
}

type triviaSnap struct {
	commentsLen          int
	processed            int
	sawNewline           bool
	sawNewlineForComment bool
	previousKind         token.Token
	previousStart        ast.Idx
}

func (t *triviaBuilder) snapshot() triviaSnap {
	return triviaSnap{
		commentsLen:          len(t.comments),
		processed:            t.processed,
		sawNewline:           t.sawNewline,
		sawNewlineForComment: t.sawNewlineForComment,
		previousKind:         t.previousKind,
		previousStart:        t.previousStart,
	}
}

func (t *triviaBuilder) restore(s triviaSnap) {
	t.comments = t.comments[:s.commentsLen]
	t.processed = s.processed
	t.sawNewline = s.sawNewline
	t.sawNewlineForComment = s.sawNewlineForComment
	t.previousKind = s.previousKind
	t.previousStart = s.previousStart
}

func (t *triviaBuilder) addComment(start, end ast.Idx, kind ast.CommentKind, src Source) {
	if last := len(t.comments); last > 0 && start <= t.comments[last-1].Start {
		return
	}

	raw := src.Slice(start, end)
	c := ast.Comment{
		Start:    start,
		End:      end,
		Kind:     kind,
		Position: ast.CommentTrailing,
		Content:  parseAnnotation(raw, kind),
	}
	if t.sawNewlineForComment {
		c.Newlines |= ast.CommentNewlinePreceded
	}

	if kind == ast.CommentLine {
		c.Newlines |= ast.CommentNewlineFollowed
		if !t.sawNewline && !isStayLeadingToken(t.previousKind) && !c.StayLeading() {
			t.markPendingTrailing()
			c.Position = ast.CommentTrailing
			c.AttachedTo = t.previousStart
			t.processed = len(t.comments) + 1
		}
		t.sawNewline = true
		t.sawNewlineForComment = true
	} else {
		t.sawNewlineForComment = false
	}

	t.comments = append(t.comments, c)
}

func (t *triviaBuilder) handleNewline() {
	n := len(t.comments)
	if t.processed < n {
		last := &t.comments[n-1]
		last.Newlines |= ast.CommentNewlineFollowed
		if !t.sawNewline && !last.StayLeading() {
			t.markPendingTrailing()
			t.processed = n
		}
	}
	t.sawNewline = true
	t.sawNewlineForComment = true
}

func (t *triviaBuilder) markPendingTrailing() {
	for i := t.processed; i < len(t.comments); i++ {
		t.comments[i].Position = ast.CommentTrailing
		t.comments[i].AttachedTo = t.previousStart
	}
}

func (t *triviaBuilder) handleToken(kind token.Token, start ast.Idx) {
	t.previousKind = kind
	t.previousStart = start
	t.sawNewline = false
	t.sawNewlineForComment = false
	if t.processed < len(t.comments) {
		t.attachPendingLeading(start)
	}
}

func (t *triviaBuilder) attachPendingLeading(start ast.Idx) {
	for i := t.processed; i < len(t.comments); i++ {
		t.comments[i].Position = ast.CommentLeading
		t.comments[i].AttachedTo = start
	}
	t.processed = len(t.comments)
}

func isStayLeadingToken(k token.Token) bool {
	switch k {
	case token.Assign, token.LeftParenthesis, token.Colon:
		return true
	}
	return false
}

func parseAnnotation(raw string, kind ast.CommentKind) ast.CommentContent {
	c := ast.Comment{Start: 0, End: ast.Idx(len(raw)), Kind: kind}
	body := c.Body(raw)
	if body == "" {
		return ast.ContentNone
	}

	if body[0] == '!' {
		return ast.ContentLegal
	}

	if kind != ast.CommentLine && body[0] == '*' {
		if allStars(body) {
			return ast.ContentNone
		}
		if containsLicenseOrPreserve(body) {
			return ast.ContentJsdocLegal
		}
		return ast.ContentJsdoc
	}

	i := 0
	for i < len(body) && isASCIISpace(body[i]) {
		i++
	}
	if i >= len(body) {
		return ast.ContentNone
	}

	if body[i] == '@' || body[i] == '#' {
		sigil := body[i]
		rest := body[i+1:]
		if strings.HasPrefix(rest, "__PURE__") {
			return ast.ContentPure
		}
		if strings.HasPrefix(rest, "__NO_SIDE_EFFECTS__") {
			return ast.ContentNoSideEffects
		}
		if sigil == '@' && (strings.HasPrefix(rest, "license") || strings.HasPrefix(rest, "preserve")) {
			return ast.ContentLegal
		}
	}

	if isDumpMeta(body[i:]) {
		return ast.ContentDumpMeta
	}
	if containsLicenseOrPreserve(body) {
		return ast.ContentLegal
	}
	return ast.ContentNone
}

func allStars(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '*' {
			return false
		}
	}
	return true
}

func containsLicenseOrPreserve(s string) bool {
	return strings.Contains(s, "@license") || strings.Contains(s, "@preserve")
}

func isDumpMeta(s string) bool {
	i := 0
	n := len(s)
	if i >= n || s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i >= n || s[i] != ' ' {
		return false
	}
	i++
	if i+3 > n || s[i] != 'p' || s[i+1] != 'c' || s[i+2] != '=' {
		return false
	}
	i += 3
	if i >= n || s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i >= n || s[i] != ' ' {
		return false
	}
	i++
	if i+3 > n || s[i] != 'd' || s[i+1] != 'k' || s[i+2] != '=' {
		return false
	}
	i += 3
	if i >= n || s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	for i < n {
		if !isASCIISpace(s[i]) {
			return false
		}
		i++
	}
	return true
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}
