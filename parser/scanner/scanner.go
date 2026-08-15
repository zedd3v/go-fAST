package scanner

import (
	"errors"
	"sync"

	"github.com/t14raptor/go-fast/ast"
	"github.com/t14raptor/go-fast/parser/scanner/token"
)

var triviaPool = sync.Pool{New: func() any {
	t := newTriviaBuilder()
	return &t
}}

type Scanner struct {
	Token Token

	src Source

	EscapedStr string // escape-processed string for current token

	errors *error

	trivia *triviaBuilder

	hasPeek   bool
	peeked    Token
	peekedEsc string
}

func NewScanner(src string, errors *error) Scanner {
	return Scanner{
		src:    NewSource(src),
		errors: errors,
	}
}

func (s *Scanner) error(d Error) {
	*s.errors = errors.Join(*s.errors, d)
}

// unexpectedErr reports an unexpected character/end error at the current position.
func (s *Scanner) unexpectedErr() {
	if s.src.EOF() {
		s.error(unexpectedEnd(s.src.Offset()))
	} else {
		b := s.src.PeekByteUnchecked()
		start := s.src.Offset()
		s.src.pos++
		s.error(invalidCharacter(rune(b), start, s.src.Offset()))
	}
}

// unterminatedRange returns the range from the current Token start to the current position.
func (s *Scanner) unterminatedRange() (ast.Idx, ast.Idx) {
	return s.Token.Idx0, s.src.Offset()
}

type Checkpoint struct {
	pos        ast.Idx
	tok        Token
	escapedStr string
	errors     error

	hasPeek   bool
	peeked    Token
	peekedEsc string

	trivia triviaSnap
}

func (s *Scanner) Checkpoint() Checkpoint {
	cp := Checkpoint{
		pos:        s.src.pos,
		tok:        s.Token,
		escapedStr: s.EscapedStr,
		errors:     *s.errors,
		hasPeek:    s.hasPeek,
		peeked:     s.peeked,
		peekedEsc:  s.peekedEsc,
	}
	if s.trivia != nil {
		cp.trivia = s.trivia.snapshot()
	}
	return cp
}

func (s *Scanner) Rewind(c Checkpoint) {
	s.src.pos = c.pos
	s.Token = c.tok
	s.EscapedStr = c.escapedStr
	*s.errors = c.errors
	s.hasPeek = c.hasPeek
	s.peeked = c.peeked
	s.peekedEsc = c.peekedEsc
	if s.trivia != nil {
		s.trivia.restore(c.trivia)
	}
}

func (s *Scanner) Next() {
	if s.hasPeek {
		s.Token = s.peeked
		s.EscapedStr = s.peekedEsc
		s.hasPeek = false
		return
	}
	s.scan()
}

func (s *Scanner) Peek() Token {
	if !s.hasPeek {
		savedTok := s.Token
		savedEsc := s.EscapedStr
		s.scan()
		s.peeked = s.Token
		s.peekedEsc = s.EscapedStr
		s.Token = savedTok
		s.EscapedStr = savedEsc
		s.hasPeek = true
	}
	return s.peeked
}

func getTrivia() *triviaBuilder {
	t := triviaPool.Get().(*triviaBuilder)
	t.Reset()
	return t
}

func (s *Scanner) putTrivia() {
	t := s.trivia
	if t == nil {
		return
	}
	s.trivia = nil
	t.comments = t.comments[:0]
	triviaPool.Put(t)
}

// CollectComments turns comment recording on or off. NewScanner starts off.
func (s *Scanner) CollectComments(on bool) {
	if !on {
		s.putTrivia()
		return
	}
	if s.trivia == nil {
		s.trivia = getTrivia()
		if cap(s.trivia.comments) < commentBufMin {
			s.trivia.comments = make([]ast.Comment, 0, commentBufMin)
		}
	}
}

func (s *Scanner) SetCommentBuf(buf []ast.Comment, srcLen int) {
	if s.trivia == nil {
		s.trivia = getTrivia()
	} else {
		s.trivia.Reset()
	}
	s.trivia.comments = sizedCommentBuf(buf, srcLen)
}

func (s *Scanner) TakeCommentBuf() []ast.Comment {
	if s.trivia == nil {
		return nil
	}
	buf := s.trivia.comments
	s.trivia.comments = nil
	s.putTrivia()
	if buf == nil {
		return nil
	}
	return buf[:0]
}

func (s *Scanner) TakeComments() []ast.Comment {
	if s.trivia == nil {
		return nil
	}
	n := len(s.trivia.comments)
	if n == 0 {
		return nil
	}
	out := s.trivia.comments
	s.trivia.comments = nil
	return out
}

func (s *Scanner) Offset() ast.Idx {
	return s.src.Offset()
}

func (s *Scanner) NextRune() (rune, bool) {
	return s.src.NextRune()
}

func (s *Scanner) NextByte() (byte, bool) {
	return s.src.NextByte()
}

func (s *Scanner) ConsumeRune() rune {
	r, _ := s.src.NextRune()
	return r
}

func (s *Scanner) ConsumeByte() byte {
	return s.src.NextByteUnchecked()
}

func (s *Scanner) PeekRune() (rune, bool) {
	return s.src.PeekRune()
}

func (s *Scanner) PeekByte() (byte, bool) {
	return s.src.PeekByte()
}

func (s *Scanner) AdvanceIfByteEquals(b byte) bool {
	return s.src.AdvanceIfByteEquals(b)
}

func (s *Scanner) NextTemplatePart() {
	s.hasPeek = false
	s.Token.Idx0 = s.src.Offset() - 1
	s.Token.Kind = s.ReadTemplateLiteral(token.TemplateMiddle, token.TemplateTail)
	s.Token.Idx1 = s.src.Offset()
}
