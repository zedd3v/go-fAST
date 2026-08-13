package ast

import (
	"testing"
	"unsafe"
)

func TestCommentSizeof(t *testing.T) {
	if unsafe.Sizeof(Comment{}) != 16 {
		t.Fatalf("Comment size = %d; want 16", unsafe.Sizeof(Comment{}))
	}
}

func TestCommentZeroPositionIsTrailing(t *testing.T) {
	var c Comment
	if c.Position != CommentTrailing {
		t.Fatalf("zero Position = %v; want Trailing", c.Position)
	}
	if !c.IsTrailing() || c.IsLeading() {
		t.Fatal("zero Comment must be trailing")
	}
}

func TestCommentBodyUnterminated(t *testing.T) {
	c := Comment{Start: 0, End: 2, Kind: CommentSingleLineBlock}
	if got := c.Body("/*"); got != "" {
		t.Fatalf("Body(/*) = %q; want %q", got, "")
	}

	c = Comment{Start: 0, End: 4, Kind: CommentSingleLineBlock}
	if got := c.Body("/* x"); got != " x" {
		t.Fatalf("Body(/* x) = %q; want %q", got, " x")
	}
}

func TestProgramCloneCopiesComments(t *testing.T) {
	p := &Program{
		Comments: []Comment{{Start: 1, End: 4}},
		Source:   "/*x*/",
	}
	cloned := p.Clone()
	if len(cloned.Comments) != 1 || cloned.Comments[0].Start != 1 {
		t.Fatalf("clone comments = %#v", cloned.Comments)
	}
	cloned.Comments[0].Start = 9
	if p.Comments[0].Start != 1 {
		t.Fatal("clone shared comment backing")
	}
	if cloned.Source != p.Source {
		t.Fatalf("clone Source = %q; want %q", cloned.Source, p.Source)
	}
}
