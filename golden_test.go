package tipping

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type scenario struct {
	name             string
	corpusPath       string
	threshold        float64
	symbols          string
	filterAlphabetic bool
	filterNumeric    bool
	filterImpure     bool
	specialWhites    []string
	specialBlacks    []string
}

type parseOutput struct {
	Clusters  []int             `json:"clusters"`
	Templates [][]string        `json:"templates"`
	Masks     map[string]string `json:"masks"`
}

type canonicalCluster struct {
	Indices   []int    `json:"indices"`
	Templates []string `json:"templates"`
}

type canonicalOutput struct {
	Clusters    []canonicalCluster `json:"clusters"`
	Unclustered []int              `json:"unclustered"`
	Masks       map[string]string  `json:"masks"`
}

type oracleInput struct {
	Threshold        float64  `json:"threshold"`
	Symbols          string   `json:"symbols"`
	FilterAlphabetic bool     `json:"filter_alphabetic"`
	FilterNumeric    bool     `json:"filter_numeric"`
	FilterImpure     bool     `json:"filter_impure"`
	SpecialWhites    []string `json:"special_whites"`
	SpecialBlacks    []string `json:"special_blacks"`
	Messages         []string `json:"messages"`
}

func TestGoldenParity(t *testing.T) {
	updateGolden := os.Getenv("TIPPING_UPDATE_GOLDEN") == "1"
	verifyGoOracle := os.Getenv("TIPPING_VERIFY_GO_ORACLE") == "1"

	scenarios := []scenario{
		{
			name:             "default",
			corpusPath:       filepath.Join("testdata", "corpus", "default.log"),
			threshold:        0.5,
			filterAlphabetic: true,
		},
		{
			name:             "special",
			corpusPath:       filepath.Join("testdata", "corpus", "special.log"),
			threshold:        0.5,
			symbols:          ".",
			filterAlphabetic: true,
			specialWhites: []string{
				`Fan`,
				`Temp`,
			},
			specialBlacks: []string{
				`\d+\.\d+`,
			},
		},
	}

	goPath, goAvailable := findGo()
	haveGoOracle := verifyGoOracle && goAvailable

	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			messages := readCorpus(t, sc.corpusPath)
			goOut := runGoParser(t, sc, messages)
			goCanonical := canonicalize(goOut)

			goldenPath := filepath.Join("testdata", "golden", sc.name+".json")

			var oracleCanonical canonicalOutput
			if haveGoOracle {
				oracleOut := runGoOracle(t, goPath, sc, messages)
				oracleCanonical = canonicalize(oracleOut)
			}

			if updateGolden {
				if haveGoOracle {
					writeJSON(t, goldenPath, oracleCanonical)
				} else {
					writeJSON(t, goldenPath, goCanonical)
				}
			}

			expected := readGolden(t, goldenPath)

			if !reflect.DeepEqual(goCanonical, expected) {
				t.Fatalf("go output drifted from golden\n got: %+v\nwant: %+v", goCanonical, expected)
			}

			if !verifyGoOracle {
				return
			}
			if !goAvailable {
				t.Skip("TIPPING_VERIFY_GO_ORACLE is enabled but go was not found in PATH")
			}

			if !reflect.DeepEqual(goCanonical, oracleCanonical) {
				t.Fatalf("go output differs from go oracle\n direct: %+v\n oracle: %+v", goCanonical, oracleCanonical)
			}
		})
	}
}

func runGoParser(t *testing.T, sc scenario, messages []string) parseOutput {
	t.Helper()
	whiteRegexes := compilePatterns(t, sc.specialWhites)
	blackRegexes := compilePatterns(t, sc.specialBlacks)

	p := NewParser().
		WithThreshold(sc.threshold).
		WithSymbols(sc.symbols).
		WithFilterAlphabetic(sc.filterAlphabetic).
		WithFilterNumeric(sc.filterNumeric).
		WithFilterImpure(sc.filterImpure).
		WithSpecialWhites(whiteRegexes).
		WithSpecialBlacks(blackRegexes)

	clusters, templates, masks := p.ParseWithTemplatesAndMasks(messages)
	return parseOutput{Clusters: clusters, Templates: templates, Masks: masks}
}

func runGoOracle(t *testing.T, goPath string, sc scenario, messages []string) parseOutput {
	t.Helper()

	payload := oracleInput{
		Threshold:        sc.threshold,
		Symbols:          sc.symbols,
		FilterAlphabetic: sc.filterAlphabetic,
		FilterNumeric:    sc.filterNumeric,
		FilterImpure:     sc.filterImpure,
		SpecialWhites:    cloneStringSlice(sc.specialWhites),
		SpecialBlacks:    cloneStringSlice(sc.specialBlacks),
		Messages:         append([]string(nil), messages...),
	}
	input, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal oracle input: %v", err)
	}

	cmd := exec.Command(goPath, "run", "./testdata/go_oracle")
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		t.Fatalf("run go oracle: %v\nstderr:\n%s", err, strings.TrimSpace(stderr.String()))
	}

	var out parseOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode go oracle output: %v\nraw:\n%s", err, strings.TrimSpace(stdout.String()))
	}
	if out.Masks == nil {
		out.Masks = map[string]string{}
	}
	if out.Templates == nil {
		out.Templates = [][]string{}
	}
	return out
}

func canonicalize(out parseOutput) canonicalOutput {
	clusterMap := make(map[int]*canonicalCluster)
	unclustered := make([]int, 0)

	for idx, cid := range out.Clusters {
		if cid < 0 {
			unclustered = append(unclustered, idx)
			continue
		}
		cluster, ok := clusterMap[cid]
		if !ok {
			templates := []string{}
			if cid < len(out.Templates) {
				templates = append([]string(nil), out.Templates[cid]...)
				sort.Strings(templates)
				templates = uniqueStrings(templates)
			}
			cluster = &canonicalCluster{Templates: templates}
			clusterMap[cid] = cluster
		}
		cluster.Indices = append(cluster.Indices, idx)
	}

	clusters := make([]canonicalCluster, 0, len(clusterMap))
	for _, cluster := range clusterMap {
		sort.Ints(cluster.Indices)
		if cluster.Templates == nil {
			cluster.Templates = []string{}
		}
		clusters = append(clusters, *cluster)
	}
	sort.Slice(clusters, func(i, j int) bool {
		if lessIntSlices(clusters[i].Indices, clusters[j].Indices) {
			return true
		}
		if lessIntSlices(clusters[j].Indices, clusters[i].Indices) {
			return false
		}
		return strings.Join(clusters[i].Templates, "\x00") < strings.Join(clusters[j].Templates, "\x00")
	})
	sort.Ints(unclustered)

	masks := make(map[string]string, len(out.Masks))
	for k, v := range out.Masks {
		masks[k] = v
	}

	return canonicalOutput{
		Clusters:    clusters,
		Unclustered: unclustered,
		Masks:       masks,
	}
}

func lessIntSlices(a, b []int) bool {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return len(a) < len(b)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for i := 1; i < len(values); i++ {
		if values[i] == values[i-1] {
			continue
		}
		out = append(out, values[i])
	}
	return out
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func findGo() (string, bool) {
	path, err := exec.LookPath("go")
	if err != nil {
		return "", false
	}
	return path, true
}

func readCorpus(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open corpus %s: %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lines := make([]string, 0, 1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read corpus %s: %v", path, err)
	}
	return lines
}

func readGolden(t *testing.T, path string) canonicalOutput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	var out canonicalOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode golden %s: %v", path, err)
	}
	if out.Masks == nil {
		out.Masks = map[string]string{}
	}
	if out.Clusters == nil {
		out.Clusters = []canonicalCluster{}
	}
	if out.Unclustered == nil {
		out.Unclustered = []int{}
	}
	return out
}

func writeJSON(t *testing.T, path string, value canonicalOutput) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func compilePatterns(t *testing.T, patterns []string) []*regexp.Regexp {
	t.Helper()
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Fatalf("compile pattern %q: %v", pattern, err)
		}
		out = append(out, re)
	}
	return out
}
