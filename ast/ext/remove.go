package ext

import "github.com/t14raptor/go-fast/ast"

// RemoveHelper is a visitor that assists in removing specific nodes from ASTs during traversal.
//
// If you override a Visit method that has deletion logic:
//   - [RemoveHelper.VisitStatements]
//   - [RemoveHelper.VisitExpressions]
//   - [RemoveHelper.VisitSequenceExpression]
//   - [RemoveHelper.VisitVariableDeclarators]
//   - [RemoveHelper.VisitVariableDeclaration]
//   - [RemoveHelper.VisitClassElements]
//   - [RemoveHelper.VisitProperties]
//
// make sure to either call the base implementation or handle removal manually.
type RemoveHelper struct {
	ast.NoopVisitor
	remove   bool
	Comments *[]ast.Comment
}

// Remove marks the current node for removal.
func (v *RemoveHelper) Remove() {
	v.remove = true
}

func (v *RemoveHelper) VisitProgram(n *ast.Program) {
	if v.Comments == nil {
		v.Comments = &n.Comments
	}
	n.VisitChildrenWith(v.V)
}

func (v *RemoveHelper) prune(lo, hi, next ast.Idx) {
	if v.Comments == nil || *v.Comments == nil {
		return
	}
	*v.Comments = ast.PruneAttached(*v.Comments, lo, hi, next)
}

func (v *RemoveHelper) VisitStatements(n *ast.Statements) {
	w := 0
	for i := 0; i < len(*n); i++ {
		(*n)[i].VisitWith(v.V)
		if v.remove {
			next := ast.Idx(0)
			if i+1 < len(*n) {
				next = (*n)[i+1].Idx0()
			}
			v.prune((*n)[i].Idx0(), (*n)[i].Idx1(), next)
			v.remove = false
			continue
		}
		if w != i {
			(*n)[w] = (*n)[i]
		}
		w++
	}
	if w == len(*n) {
		return
	}

	clear((*n)[w:])
	*n = (*n)[:w]
}

func (v *RemoveHelper) VisitExpressions(n *ast.Expressions) {
	w := 0
	for i := 0; i < len(*n); i++ {
		(*n)[i].VisitWith(v.V)
		if v.remove {
			next := ast.Idx(0)
			if i+1 < len(*n) {
				next = (*n)[i+1].Idx0()
			}
			v.prune((*n)[i].Idx0(), (*n)[i].Idx1(), next)
			v.remove = false
			continue
		}
		if w != i {
			(*n)[w] = (*n)[i]
		}
		w++
	}
	if w == len(*n) {
		return
	}

	clear((*n)[w:])
	*n = (*n)[:w]
}

func (v *RemoveHelper) VisitSequenceExpression(n *ast.SequenceExpression) {
	n.VisitChildrenWith(v.V)
	if len(n.Sequence) == 0 {
		v.Remove()
	}
}

func (v *RemoveHelper) VisitVariableDeclarators(n *ast.VariableDeclarators) {
	w := 0
	for i := 0; i < len(*n); i++ {
		(*n)[i].VisitWith(v.V)
		if v.remove {
			next := ast.Idx(0)
			if i+1 < len(*n) {
				next = (*n)[i+1].Idx0()
			}
			v.prune((*n)[i].Idx0(), (*n)[i].Idx1(), next)
			v.remove = false
			continue
		}
		if w != i {
			(*n)[w] = (*n)[i]
		}
		w++
	}
	if w == len(*n) {
		return
	}

	clear((*n)[w:])
	*n = (*n)[:w]
}

func (v *RemoveHelper) VisitVariableDeclaration(n *ast.VariableDeclaration) {
	n.VisitChildrenWith(v.V)
	if len(n.List) == 0 {
		v.Remove()
	}
}

func (v *RemoveHelper) VisitClassElements(n *ast.ClassElements) {
	w := 0
	for i := 0; i < len(*n); i++ {
		(*n)[i].VisitWith(v.V)
		if v.remove {
			next := ast.Idx(0)
			if i+1 < len(*n) {
				next = (*n)[i+1].Idx0()
			}
			v.prune((*n)[i].Idx0(), (*n)[i].Idx1(), next)
			v.remove = false
			continue
		}
		if w != i {
			(*n)[w] = (*n)[i]
		}
		w++
	}
	if w == len(*n) {
		return
	}

	clear((*n)[w:])
	*n = (*n)[:w]
}

func (v *RemoveHelper) VisitProperties(n *ast.Properties) {
	w := 0
	for i := 0; i < len(*n); i++ {
		(*n)[i].VisitWith(v.V)
		if v.remove {
			next := ast.Idx(0)
			if i+1 < len(*n) {
				next = (*n)[i+1].Idx0()
			}
			v.prune((*n)[i].Idx0(), (*n)[i].Idx1(), next)
			v.remove = false
			continue
		}
		if w != i {
			(*n)[w] = (*n)[i]
		}
		w++
	}
	if w == len(*n) {
		return
	}

	clear((*n)[w:])
	*n = (*n)[:w]
}
