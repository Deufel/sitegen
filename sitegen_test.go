package sitegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFromSource(t *testing.T) {
	out := t.TempDir()
	site, err := Build(Config{Root: "testdata/mod", Out: out, Title: "thing", Repo: "https://example.com/r", Ref: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(site.Packages) != 1 || site.Packages[0].ImportPath != "example.com/thing/thing" {
		t.Fatalf("packages: %+v", site.Packages)
	}
	want := map[string][]string{
		"index.html":       {"A thing that adds", `href="pkg/thing.html"`, `href="guide/start.html"`},
		"guide/start.html": {"Getting started", `href="../pkg/thing.html#add"`, `aria-current="page"`},
		"pkg/thing.html":   {"Package thing adds numbers", ">Why</h3>", `id="add"`, "func Add(a, b int) int", "answers a plus b", `id="counter"`, "Counter.Inc", "func (c *Counter) Inc() int", `id="examples"`, "thing.Add(2, 3)", "<code>5</code>", "Counter — inc", "A counter counts from one", "Output"},
	}
	for path, needles := range want {
		b, err := os.ReadFile(filepath.Join(out, path))
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range needles {
			if !strings.Contains(string(b), n) {
				t.Errorf("%s lacks %q", path, n)
			}
		}
	}
	if err := Must(CheckLinks(out)); err != nil {
		t.Fatal(err)
	}
	ix, err := os.ReadFile(filepath.Join(out, "search.json"))
	if err != nil || !strings.Contains(string(ix), `"T":"Counter.Inc","K":"method","H":"pkg/thing.html#counter-inc","W":"thing"`) {
		t.Fatalf("the search index: %v %s", err, ix)
	}
}

func TestCheckReportsTheGaps(t *testing.T) {
	problems, err := Check("testdata/mod")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range problems {
		got = append(got, p.Kind+" "+p.Name)
	}
	// a constructor is covered by its type's example; Add and Counter have theirs
	want := []string{"symbol-doc Undocumented", "example Undocumented"}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("check misses %q in %v", w, got)
		}
	}
	for _, g := range got {
		if strings.Contains(g, " Add") || strings.Contains(g, " Counter") && !strings.Contains(g, "NewCounter") {
			t.Errorf("check wrongly flags %q", g)
		}
	}
}
