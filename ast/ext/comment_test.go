package ext_test

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/ast"
	"github.com/t14raptor/go-fast/ast/ext"
	"github.com/t14raptor/go-fast/generator"
	"github.com/t14raptor/go-fast/parser"
)

// dropFirstStmt deletes the first statement. RemoveHelper prunes comments
// attached to the removed span; legal comments are retargeted to the next stmt.
type dropFirstStmt struct {
	ext.RemoveHelper
	dropped bool
}

func (v *dropFirstStmt) VisitStatement(n *ast.Statement) {
	if !v.dropped {
		v.dropped = true
		v.Remove()
		return
	}
	n.VisitChildrenWith(v.V)
}

func TestRemoveHelperDropsDumpMetaKeepsLegalOrphan(t *testing.T) {
	src := "/* 7355685938729369933 pc=114796 dk=5 */ /*! license */ var dead = 1;\nvar keep = 2;"
	p, err := parser.ParseWithOptions(src, parser.Options{Comments: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Body) != 2 {
		t.Fatalf("stmt count = %d; want 2", len(p.Body))
	}

	v := &dropFirstStmt{}
	v.V = v
	p.VisitWith(v)

	if len(p.Body) != 1 {
		t.Fatalf("stmt count after remove = %d; want 1", len(p.Body))
	}
	if len(p.Comments) != 1 {
		t.Fatalf("comment table = %d; want 1 (dump dropped)", len(p.Comments))
	}
	if !p.Comments[0].IsLegal() {
		t.Fatalf("kept comment is not legal: %#v", p.Comments[0])
	}
	if p.Comments[0].AttachedTo != p.Body[0].Idx0() {
		t.Fatalf("legal AttachedTo = %d; want next stmt %d", p.Comments[0].AttachedTo, p.Body[0].Idx0())
	}

	got := generator.GenerateWithOptions(p, generator.Options{Comments: true})
	if strings.Contains(got, "7355685938729369933") || strings.Contains(got, "pc=") {
		t.Fatalf("DumpMeta orphan printed: %q", got)
	}
	if !strings.Contains(got, "/*! license */") {
		t.Fatalf("legal orphan dropped: %q", got)
	}
	if !strings.Contains(got, "var keep = 2") {
		t.Fatalf("kept statement missing: %q", got)
	}
}
