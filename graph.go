package tipping

type anchorScratch struct {
	nodes     []Token
	nodeIndex map[tokenKey]int
	outDeg    []int
	inDeg     []int
	adj       [][]int
	rev       [][]int
	order     []int
	visited   []bool
	comp      []int
	largest   []int
	anchors   map[tokenKey]Token
}

func newAnchorScratch() *anchorScratch {
	return &anchorScratch{
		nodeIndex: make(map[tokenKey]int),
		anchors:   make(map[tokenKey]Token),
	}
}

func anchorTokens(tokens []Token, idep *tokenRecord, threshold float64, scratch *anchorScratch) map[tokenKey]Token {
	nodes := scratch.nodes[:0]
	nodeIndex := scratch.nodeIndex
	clear(nodeIndex)
	for _, tok := range tokens {
		if _, ok := idep.occurrence(tok.Slice); !ok {
			continue
		}
		id := tokenKeyFor(tok)
		if _, ok := nodeIndex[id]; ok {
			continue
		}
		nodeIndex[id] = len(nodes)
		nodes = append(nodes, tok)
	}
	scratch.nodes = nodes

	n := len(nodes)
	anchors := scratch.anchors
	clear(anchors)
	if n == 0 {
		return anchors
	}
	if n == 1 {
		anchors[tokenKeyFor(nodes[0])] = nodes[0]
		for _, tok := range tokens {
			switch tok.Kind {
			case TokenSpecialWhite:
				anchors[tokenKeyFor(tok)] = tok
			case TokenSpecialBlack:
				delete(anchors, tokenKeyFor(tok))
			}
		}
		return anchors
	}

	outDeg := ensureIntBuffer(scratch.outDeg, n)
	inDeg := ensureIntBuffer(scratch.inDeg, n)
	clear(outDeg)
	clear(inDeg)
	scratch.outDeg = outDeg
	scratch.inDeg = inDeg

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if dep, ok := idep.dependency(nodes[i].Slice, nodes[j].Slice); ok && dep > threshold {
				outDeg[i]++
				inDeg[j]++
			}
			if dep, ok := idep.dependency(nodes[j].Slice, nodes[i].Slice); ok && dep > threshold {
				outDeg[j]++
				inDeg[i]++
			}
		}
	}

	adj := ensureEdgeBuffer(scratch.adj, n)
	rev := ensureEdgeBuffer(scratch.rev, n)
	scratch.adj = adj
	scratch.rev = rev
	for i := 0; i < n; i++ {
		if outDeg[i] > 0 {
			if cap(adj[i]) < outDeg[i] {
				adj[i] = make([]int, 0, outDeg[i])
			}
		}
		if inDeg[i] > 0 {
			if cap(rev[i]) < inDeg[i] {
				rev[i] = make([]int, 0, inDeg[i])
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if dep, ok := idep.dependency(nodes[i].Slice, nodes[j].Slice); ok && dep > threshold {
				adj[i] = append(adj[i], j)
				rev[j] = append(rev[j], i)
			}
			if dep, ok := idep.dependency(nodes[j].Slice, nodes[i].Slice); ok && dep > threshold {
				adj[j] = append(adj[j], i)
				rev[i] = append(rev[i], j)
			}
		}
	}

	order := scratch.order[:0]
	visited := ensureBoolBuffer(scratch.visited, n)
	clear(visited)
	var dfs1 func(int)
	dfs1 = func(v int) {
		visited[v] = true
		for _, to := range adj[v] {
			if !visited[to] {
				dfs1(to)
			}
		}
		order = append(order, v)
	}
	for v := 0; v < n; v++ {
		if !visited[v] {
			dfs1(v)
		}
	}
	scratch.order = order

	clear(visited)
	largest := scratch.largest[:0]
	var dfs2 func(int, *[]int)
	dfs2 = func(v int, comp *[]int) {
		visited[v] = true
		*comp = append(*comp, v)
		for _, to := range rev[v] {
			if !visited[to] {
				dfs2(to, comp)
			}
		}
	}
	for i := len(order) - 1; i >= 0; i-- {
		v := order[i]
		if visited[v] {
			continue
		}
		comp := scratch.comp[:0]
		dfs2(v, &comp)
		scratch.comp = comp
		if len(comp) > len(largest) {
			largest = append(largest[:0], comp...)
		}
	}
	scratch.largest = largest
	scratch.visited = visited

	for _, idx := range largest {
		tok := nodes[idx]
		anchors[tokenKeyFor(tok)] = tok
	}

	for _, tok := range tokens {
		switch tok.Kind {
		case TokenSpecialWhite:
			anchors[tokenKeyFor(tok)] = tok
		case TokenSpecialBlack:
			delete(anchors, tokenKeyFor(tok))
		}
	}

	return anchors
}

func ensureIntBuffer(buf []int, n int) []int {
	if cap(buf) < n {
		return make([]int, n)
	}
	return buf[:n]
}

func ensureBoolBuffer(buf []bool, n int) []bool {
	if cap(buf) < n {
		return make([]bool, n)
	}
	return buf[:n]
}

func ensureEdgeBuffer(buf [][]int, n int) [][]int {
	if cap(buf) < n {
		buf = make([][]int, n)
	} else {
		buf = buf[:n]
	}
	for i := range buf {
		buf[i] = buf[i][:0]
	}
	return buf
}
