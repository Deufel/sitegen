package sitegen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Problem is one thing the module's documentation lacks.
type Problem struct {
	Package string
	Kind    string // "package-doc" · "symbol-doc" · "example" · "link"
	Name    string
}

// String reads "package: kind name".
func (p Problem) String() string { return p.Package + ": " + p.Kind + " " + p.Name }

// Check reads the module and reports what the discipline asks for and
// the source lacks: a package without a comment, an exported symbol
// without one, a package-level function with no example. A test asserts
// the list is empty; the strictness is the caller's — filter by Kind for
// the level a project keeps.
func Check(root string) ([]Problem, error) {
	pkgs, err := Packages(root)
	if err != nil {
		return nil, err
	}
	var out []Problem
	for _, p := range pkgs {
		if p.Name == "main" {
			if strings.TrimSpace(p.Doc) == "" {
				out = append(out, Problem{p.ImportPath, "package-doc", "main"})
			}
			continue
		}
		if strings.TrimSpace(p.Doc) == "" {
			out = append(out, Problem{p.ImportPath, "package-doc", p.Name})
		}
		exampled := map[string]bool{}
		for _, ex := range p.Examples {
			exampled[ex.For] = true
		}
		need := func(s Symbol) {
			if strings.TrimSpace(s.Doc) == "" {
				out = append(out, Problem{p.ImportPath, "symbol-doc", s.Name})
			}
		}
		for _, s := range p.Consts {
			need(s)
		}
		for _, s := range p.Vars {
			need(s)
		}
		for _, s := range p.Funcs {
			need(s)
			if !exampled[s.Name] {
				out = append(out, Problem{p.ImportPath, "example", s.Name})
			}
		}
		for _, t := range p.Types {
			need(t.Symbol) // a type is documented; its functions carry the examples
			for _, f := range t.Funcs {
				need(f)
				// go/doc files a function under the type it returns; it still
				// wants an example unless the type's own example shows it
				if !exampled[f.Name] && !exampled[t.Name] {
					out = append(out, Problem{p.ImportPath, "example", f.Name})
				}
			}
			for _, m := range t.Methods {
				m.Name = t.Name + "." + m.Name
				need(m)
			}
		}
	}
	return out, nil
}

var hrefRe = regexp.MustCompile(`href="([^"#]+)(#[^"]*)?"`)

// CheckLinks walks the built site and reports every relative link that
// names a file the site lacks.
func CheckLinks(out string) ([]Problem, error) {
	var problems []Problem
	err := filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range hrefRe.FindAllStringSubmatch(string(b), -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), filepath.FromSlash(target))); err != nil {
				rel, _ := filepath.Rel(out, path)
				problems = append(problems, Problem{rel, "link", target})
			}
		}
		return nil
	})
	return problems, err
}

// Must fails when problems remain — the one-line guard for a test.
func Must(problems []Problem, err error) error {
	if err != nil {
		return err
	}
	if len(problems) == 0 {
		return nil
	}
	var b strings.Builder
	for _, p := range problems {
		fmt.Fprintf(&b, "%s\n", p)
	}
	return fmt.Errorf("sitegen: %d problems\n%s", len(problems), b.String())
}
