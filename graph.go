package tipping

type dfsFrame struct {
	v, i int
}

type anchorScratch struct {
	nodes     []Token
	nodeEpoch []uint32 // per token id: last epoch the id produced a node
	nodeKinds []uint8  // per token id: kind bits seen in that epoch
	epoch     uint32
	tokenIDs  []uint32
	occThresh []float64
	adj       [][]int
	rev       [][]int
	order     []int
	visited   []bool
	comp      []int
	largest   []int
	frames    []dfsFrame
	stack     []int
	anchors   map[tokenKey]Token
}

func newAnchorScratch(numIDs int) *anchorScratch {
	return &anchorScratch{
		nodeEpoch: make([]uint32, numIDs),
		nodeKinds: make([]uint8, numIDs),
		anchors:   make(map[tokenKey]Token),
	}
}

func anchorTokens(tokens []Token, tokenIDs []uint32, idep *tokenRecord, threshold float64, scratch *anchorScratch) map[tokenKey]Token {
	nodes := scratch.nodes[:0]
	ids := scratch.tokenIDs[:0]
	scratch.epoch++
	epoch := scratch.epoch
	for j, tok := range tokens {
		id := tokenIDs[j]
		if idep.occ[id] == 0 {
			continue
		}
		if scratch.nodeEpoch[id] != epoch {
			scratch.nodeEpoch[id] = epoch
			scratch.nodeKinds[id] = 0
		}
		bit := uint8(1) << tok.Kind
		if scratch.nodeKinds[id]&bit != 0 {
			continue
		}
		scratch.nodeKinds[id] |= bit
		nodes = append(nodes, tok)
		ids = append(ids, id)
	}
	scratch.nodes = nodes
	scratch.tokenIDs = ids

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

	adj := ensureEdgeBuffer(scratch.adj, n)
	rev := ensureEdgeBuffer(scratch.rev, n)
	scratch.adj = adj
	scratch.rev = rev

	occThresh := scratch.occThresh
	if cap(occThresh) < n {
		occThresh = make([]float64, n)
	} else {
		occThresh = occThresh[:n]
	}
	for i := range occThresh {
		occThresh[i] = float64(idep.occ[int(ids[i])]) * threshold
	}
	scratch.occThresh = occThresh

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			co := idep.co[newTokenPairID(ids[i], ids[j])]
			if co == 0 {
				continue
			}
			cof := float64(co)
			if cof > occThresh[i] {
				adj[i] = append(adj[i], j)
				rev[j] = append(rev[j], i)
			}
			if cof > occThresh[j] {
				adj[j] = append(adj[j], i)
				rev[i] = append(rev[i], j)
			}
		}
	}

	// Kosaraju pass 1: postorder over adj, iterative to avoid closure and
	// stack-growth allocations on this per-row hot path.
	order := scratch.order[:0]
	visited := ensureBoolBuffer(scratch.visited, n)
	clear(visited)
	frames := scratch.frames[:0]
	for v0 := 0; v0 < n; v0++ {
		if visited[v0] {
			continue
		}
		visited[v0] = true
		frames = append(frames, dfsFrame{v: v0})
		for len(frames) > 0 {
			f := &frames[len(frames)-1]
			if f.i < len(adj[f.v]) {
				to := adj[f.v][f.i]
				f.i++
				if !visited[to] {
					visited[to] = true
					frames = append(frames, dfsFrame{v: to})
				}
				continue
			}
			order = append(order, f.v)
			frames = frames[:len(frames)-1]
		}
	}
	scratch.order = order
	scratch.frames = frames

	// Kosaraju pass 2: collect components over rev; only component sizes and
	// membership matter, so visit order within a component is irrelevant.
	clear(visited)
	largest := scratch.largest[:0]
	stack := scratch.stack
	for i := len(order) - 1; i >= 0; i-- {
		v := order[i]
		if visited[v] {
			continue
		}
		comp := scratch.comp[:0]
		visited[v] = true
		stack = append(stack[:0], v)
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			comp = append(comp, x)
			for _, to := range rev[x] {
				if !visited[to] {
					visited[to] = true
					stack = append(stack, to)
				}
			}
		}
		scratch.comp = comp
		if len(comp) > len(largest) {
			largest = append(largest[:0], comp...)
		}
	}
	scratch.stack = stack
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
