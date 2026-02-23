package tipping

type ahoCorasick struct {
	nodes []ahoNode
}

type ahoNode struct {
	next [256]int32
	fail int32
	out  []int
}

func newAhoCorasick(patterns []string) *ahoCorasick {
	if len(patterns) == 0 {
		return nil
	}

	a := &ahoCorasick{
		nodes: []ahoNode{newAhoNode()},
	}
	for id, pattern := range patterns {
		if pattern == "" {
			continue
		}
		state := int32(0)
		for i := 0; i < len(pattern); i++ {
			b := pattern[i]
			next := a.nodes[state].next[b]
			if next < 0 {
				next = int32(len(a.nodes))
				a.nodes[state].next[b] = next
				a.nodes = append(a.nodes, newAhoNode())
			}
			state = next
		}
		a.nodes[state].out = append(a.nodes[state].out, id)
	}
	a.buildFailures()
	return a
}

func newAhoNode() ahoNode {
	n := ahoNode{}
	for i := range n.next {
		n.next[i] = -1
	}
	return n
}

func (a *ahoCorasick) buildFailures() {
	queue := make([]int32, 0, len(a.nodes))

	root := &a.nodes[0]
	for b := 0; b < len(root.next); b++ {
		next := root.next[b]
		if next < 0 {
			continue
		}
		a.nodes[next].fail = 0
		queue = append(queue, next)
	}

	for head := 0; head < len(queue); head++ {
		state := queue[head]
		node := &a.nodes[state]
		for b := 0; b < len(node.next); b++ {
			next := node.next[b]
			if next < 0 {
				continue
			}
			queue = append(queue, next)

			fail := node.fail
			for fail != 0 && a.nodes[fail].next[b] < 0 {
				fail = a.nodes[fail].fail
			}
			if fallback := a.nodes[fail].next[b]; fallback >= 0 {
				a.nodes[next].fail = fallback
			} else {
				a.nodes[next].fail = 0
			}

			failOut := a.nodes[a.nodes[next].fail].out
			if len(failOut) > 0 {
				a.nodes[next].out = append(a.nodes[next].out, failOut...)
			}
		}
	}
}

// Search walks text and invokes visit for each matched pattern id.
// Returning false from visit stops the scan early.
func (a *ahoCorasick) Search(text string, visit func(patternID int) bool) {
	if a == nil || len(text) == 0 {
		return
	}

	state := int32(0)
	for i := 0; i < len(text); i++ {
		b := text[i]
		for state != 0 && a.nodes[state].next[b] < 0 {
			state = a.nodes[state].fail
		}
		if next := a.nodes[state].next[b]; next >= 0 {
			state = next
		} else {
			state = 0
		}

		out := a.nodes[state].out
		for _, id := range out {
			if !visit(id) {
				return
			}
		}
	}
}
