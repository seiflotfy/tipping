package tipping

func anchorTokens(tokens []Token, idep *tokenRecord, threshold float64) map[string]Token {
	nodes := make([]Token, 0, len(tokens))
	nodeIndex := make(map[string]int)
	for _, tok := range tokens {
		if _, ok := idep.occurrence(tok.Slice); !ok {
			continue
		}
		id := tokenIdentity(tok)
		if _, ok := nodeIndex[id]; ok {
			continue
		}
		nodeIndex[id] = len(nodes)
		nodes = append(nodes, tok)
	}

	n := len(nodes)
	if n == 0 {
		return map[string]Token{}
	}

	adj := make([][]int, n)
	rev := make([][]int, n)
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

	order := make([]int, 0, n)
	visited := make([]bool, n)
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

	for i := range visited {
		visited[i] = false
	}
	largest := []int{}
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
		comp := make([]int, 0, 4)
		dfs2(v, &comp)
		if len(comp) > len(largest) {
			largest = comp
		}
	}

	anchors := make(map[string]Token, len(largest))
	for _, idx := range largest {
		tok := nodes[idx]
		anchors[tokenIdentity(tok)] = tok
	}

	for _, tok := range tokens {
		switch tok.Kind {
		case TokenSpecialWhite:
			anchors[tokenIdentity(tok)] = tok
		case TokenSpecialBlack:
			delete(anchors, tokenIdentity(tok))
		}
	}

	return anchors
}
