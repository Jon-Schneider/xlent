package engine

import "sort"

// Graph tracks which cells each formula cell references, and answers the
// reverse question: when a cell changes, which formula cells must be
// re-evaluated, and in what order.
//
// Dependent lookup scans every registered formula's references (O(formulas ×
// refs) per edit). That holds up well into the tens of thousands of formulas;
// an interval index can replace the scan without changing this API if it ever
// becomes hot.
type Graph struct {
	deps map[Node][]Ref
}

func NewGraph() *Graph {
	return &Graph{deps: make(map[Node][]Ref)}
}

// Set records that formula cell n references refs, replacing any previous
// registration for n.
func (g *Graph) Set(n Node, refs []Ref) {
	if len(refs) == 0 {
		delete(g.deps, n)
		return
	}
	g.deps[n] = refs
}

// Remove forgets the formula registered at n (e.g. the cell was cleared or
// overwritten with a plain value).
func (g *Graph) Remove(n Node) {
	delete(g.deps, n)
}

// IsFormula reports whether n is registered as a formula cell with references.
func (g *Graph) IsFormula(n Node) bool {
	_, ok := g.deps[n]
	return ok
}

// directDependents returns the formula cells that reference n directly,
// sorted for deterministic traversal order.
func (g *Graph) directDependents(n Node) []Node {
	var out []Node
	for formula, refs := range g.deps {
		for _, r := range refs {
			if r.Contains(n) {
				out = append(out, formula)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Sheet != b.Sheet {
			return a.Sheet < b.Sheet
		}
		if a.Row != b.Row {
			return a.Row < b.Row
		}
		return a.Col < b.Col
	})
	return out
}

// Invalidate computes the consequences of a change to cell changed: the
// formula cells that need re-evaluation in dependency order (a cell appears
// only after everything it depends on within the dirty set), and the set of
// cells caught in a reference cycle, which must not be evaluated at all.
//
// The changed cell itself is never included in order; callers re-evaluate it
// as part of applying the edit. It does appear in cycles if it references
// itself directly or transitively.
func (g *Graph) Invalidate(changed Node) (order []Node, cycles map[Node]bool) {
	const (
		unvisited = 0
		onStack   = 1
		done      = 2
	)
	state := make(map[Node]int)
	cycles = make(map[Node]bool)
	var stack []Node
	var postorder []Node

	var walk func(n Node)
	walk = func(n Node) {
		state[n] = onStack
		stack = append(stack, n)

		for _, dep := range g.directDependents(n) {
			switch state[dep] {
			case unvisited:
				walk(dep)
			case onStack:
				// Found a cycle: everything on the stack from dep onward is
				// part of it.
				for i := len(stack) - 1; i >= 0; i-- {
					cycles[stack[i]] = true
					if stack[i] == dep {
						break
					}
				}
			}
		}

		stack = stack[:len(stack)-1]
		state[n] = done
		postorder = append(postorder, n)
	}
	walk(changed)

	// Reverse postorder is topological order. Drop the changed cell and any
	// cycle members; cycle members are reported, not evaluated.
	for i := len(postorder) - 1; i >= 0; i-- {
		n := postorder[i]
		if n == changed || cycles[n] {
			continue
		}
		order = append(order, n)
	}
	return order, cycles
}
