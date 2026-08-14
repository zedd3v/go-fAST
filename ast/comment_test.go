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

func TestLeadingTrailingFilterByAttachedTo(t *testing.T) {
	cs := []Comment{
		{Start: 0, End: 4, AttachedTo: 10, Position: CommentLeading},
		{Start: 20, End: 24, AttachedTo: 10, Position: CommentTrailing},
		{Start: 30, End: 34, AttachedTo: 40, Position: CommentLeading},
		{Start: 50, End: 54, AttachedTo: 40, Position: CommentTrailing},
		{Start: 60, End: 64, AttachedTo: 10, Position: CommentLeading},
	}

	var buf [8]Comment
	lead := Leading(cs, 10, buf[:0])
	if len(lead) != 2 || lead[0].Start != 0 || lead[1].Start != 60 {
		t.Fatalf("Leading(10) = %#v; want Start 0 then 60", lead)
	}
	trail := Trailing(cs, 10, buf[:0])
	if len(trail) != 1 || trail[0].Start != 20 {
		t.Fatalf("Trailing(10) = %#v; want Start 20", trail)
	}
	if got := Leading(cs, 40, buf[:0]); len(got) != 1 || got[0].Start != 30 {
		t.Fatalf("Leading(40) = %#v; want Start 30", got)
	}
	if got := Trailing(cs, 40, buf[:0]); len(got) != 1 || got[0].Start != 50 {
		t.Fatalf("Trailing(40) = %#v; want Start 50", got)
	}
	if got := Leading(cs, 99, buf[:0]); len(got) != 0 {
		t.Fatalf("Leading(99) = %#v; want empty", got)
	}
	if got := Trailing(cs, 99, buf[:0]); len(got) != 0 {
		t.Fatalf("Trailing(99) = %#v; want empty", got)
	}
}

func TestMoveRetargetsAttachedTo(t *testing.T) {
	cs := []Comment{
		{Start: 0, End: 4, AttachedTo: 10, Position: CommentLeading},
		{Start: 20, End: 24, AttachedTo: 10, Position: CommentTrailing},
		{Start: 30, End: 34, AttachedTo: 40, Position: CommentLeading},
	}

	Move(cs, 10, 80)

	if cs[0].AttachedTo != 80 || cs[1].AttachedTo != 80 {
		t.Fatalf("Move did not retarget 10 → 80: %#v", cs[:2])
	}
	if cs[2].AttachedTo != 40 {
		t.Fatalf("Move retargeted unrelated comment: %#v", cs[2])
	}
	var buf [8]Comment
	if got := Leading(cs, 80, buf[:0]); len(got) != 1 || got[0].Start != 0 {
		t.Fatalf("Leading after Move = %#v; want Start 0", got)
	}
	if got := Trailing(cs, 80, buf[:0]); len(got) != 1 || got[0].Start != 20 {
		t.Fatalf("Trailing after Move = %#v; want Start 20", got)
	}
	if got := Leading(cs, 10, buf[:0]); len(got) != 0 {
		t.Fatalf("Leading(old) after Move = %#v; want empty", got)
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
