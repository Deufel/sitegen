package sitegen

import (
	"bytes"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Package is one Go package as the site reads it.
type Package struct {
	ImportPath string
	Name       string
	Dir        string   // relative to the module root
	Doc        string   // the package comment, raw
	Synopsis   string   // its first sentence
	Consts     []Symbol // exported, grouped as declared
	Vars       []Symbol
	Types      []Type
	Funcs      []Symbol // package-level functions
	Examples   []Example
}

// Symbol is one exported declaration: its printed signature and its comment.
type Symbol struct {
	Name string
	Sig  string // the declaration as printed, bodies stripped
	Doc  string
}

// Type is an exported type with the functions and methods that belong to it.
type Type struct {
	Symbol
	Funcs   []Symbol // constructors: functions returning the type
	Methods []Symbol
}

// Example is one Example function: what it shows, its code, its output.
type Example struct {
	Name   string // the example's full name (ExampleRun_backup)
	For    string // the symbol it documents ("" = the package)
	Suffix string // the part after the underscore
	Doc    string
	Code   string
	Output string
}

// Packages reads every package under root (the module's directory):
// directories holding non-test Go files, except testdata, vendor, hidden
// and internal ones. The import path is the module path joined with the
// directory.
func Packages(root string) ([]Package, error) {
	modPath, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	var out []Package
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		base := d.Name()
		if path != root && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "testdata" || base == "vendor" || base == "internal") {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			rel = ""
		}
		pkg, ok, err := readPackage(path, rel, modPath)
		if err != nil {
			return err
		}
		if ok {
			out = append(out, pkg)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ImportPath < out[j].ImportPath })
	return out, err
}

func modulePath(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", os.ErrNotExist
}

func readPackage(dir, rel, modPath string) (Package, bool, error) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Package{}, false, err
	}
	var files, tests []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return Package{}, false, err
		}
		if strings.HasSuffix(name, "_test.go") {
			tests = append(tests, f)
		} else {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return Package{}, false, nil
	}
	importPath := modPath
	if rel != "" {
		importPath = modPath + "/" + filepath.ToSlash(rel)
	}
	dp, err := doc.NewFromFiles(fset, files, importPath)
	if err != nil {
		return Package{}, false, err
	}
	pkg := Package{ImportPath: importPath, Name: dp.Name, Dir: rel, Doc: dp.Doc, Synopsis: dp.Synopsis(dp.Doc)}
	sig := func(node any) string {
		var b bytes.Buffer
		_ = printer.Fprint(&b, fset, node)
		return b.String()
	}
	for _, v := range dp.Consts {
		pkg.Consts = append(pkg.Consts, Symbol{Name: strings.Join(v.Names, ", "), Sig: sig(v.Decl), Doc: v.Doc})
	}
	for _, v := range dp.Vars {
		pkg.Vars = append(pkg.Vars, Symbol{Name: strings.Join(v.Names, ", "), Sig: sig(v.Decl), Doc: v.Doc})
	}
	funcSig := func(f *doc.Func) Symbol {
		decl := *f.Decl
		decl.Body = nil
		decl.Doc = nil
		return Symbol{Name: f.Name, Sig: sig(&decl), Doc: f.Doc}
	}
	for _, t := range dp.Types {
		decl := *t.Decl
		decl.Doc = nil
		ty := Type{Symbol: Symbol{Name: t.Name, Sig: sig(&decl), Doc: t.Doc}}
		for _, f := range t.Funcs {
			ty.Funcs = append(ty.Funcs, funcSig(f))
		}
		for _, m := range t.Methods {
			ty.Methods = append(ty.Methods, funcSig(m))
		}
		pkg.Types = append(pkg.Types, ty)
	}
	for _, f := range dp.Funcs {
		pkg.Funcs = append(pkg.Funcs, funcSig(f))
	}
	for _, ex := range doc.Examples(tests...) {
		var b bytes.Buffer
		if ex.Play != nil {
			_ = printer.Fprint(&b, fset, ex.Play)
		} else {
			_ = printer.Fprint(&b, fset, ex.Code)
		}
		for_, suffix := splitExample(ex.Name)
		pkg.Examples = append(pkg.Examples, Example{Name: "Example" + suffixName(ex.Name), For: for_, Suffix: suffix, Doc: ex.Doc, Code: trimBlock(b.String()), Output: strings.TrimSpace(ex.Output)})
	}
	return pkg, true, nil
}

// splitExample — doc.Example.Name is "Run" or "Run_backup" or "" or
// "_suffix" (the package example); the symbol and the suffix.
func splitExample(name string) (string, string) {
	sym, suffix, _ := strings.Cut(name, "_")
	return sym, suffix
}

func suffixName(name string) string {
	if name == "" {
		return ""
	}
	return name
}

// trimBlock — a printed example body arrives as "{\n\t...\n}"; the braces
// and one level of indent go.
func trimBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		s = strings.TrimSpace(s[1 : len(s)-1])
		var lines []string
		for _, l := range strings.Split(s, "\n") {
			lines = append(lines, strings.TrimPrefix(l, "\t"))
		}
		s = strings.Join(lines, "\n")
	}
	return s
}

// DocHTML renders a Go doc comment (the go/doc/comment syntax: headings,
// lists, code blocks, links) to HTML.
func DocHTML(text string) string {
	var p comment.Parser
	d := p.Parse(text)
	var pr comment.Printer
	pr.HeadingLevel = 3
	return string(pr.HTML(d))
}
