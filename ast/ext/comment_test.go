package ext_test

import (
	"strings"
	"testing"

	"github.com/t14raptor/go-fast/ast"
	"github.com/t14raptor/go-fast/ast/ext"
	"github.com/t14raptor/go-fast/generator"
	"github.com/t14raptor/go-fast/parser"
)

// dropFirstStmt deletes the first statement. RemoveHelper does not touch
// Program.Comments, so those comments become orphans.
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
	p, err := parser.Parse(src)
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
	if len(p.Comments) != 2 {
		t.Fatalf("RemoveHelper rewrote comments: %d; want 2", len(p.Comments))
	}

	got := generator.Generate(p)
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
