package fixture

type node struct{ kids []*node }

// Class H: a depth cap that makes legitimate deep inputs fail.
func walk(n *node, depth int) int {
	if depth > 16 { // want H
		return 0
	}
	t := 1
	for _, k := range n.kids {
		t += walk(k, depth+1)
	}
	return t
}
