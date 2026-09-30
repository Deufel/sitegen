package sitegen_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/Deufel/sitegen"
)

func ExamplePackages() {
	pkgs, _ := sitegen.Packages("testdata/mod")
	for _, p := range pkgs {
		fmt.Println(p.ImportPath, "—", p.Synopsis, "—", len(p.Funcs), "functions,", len(p.Examples), "examples")
	}
	// Output: example.com/thing/thing — Package thing adds numbers. — 2 functions, 2 examples
}

func ExampleBuild() {
	out, _ := os.MkdirTemp("", "site")
	site, _ := sitegen.Build(sitegen.Config{Root: "testdata/mod", Out: out, Title: "thing"})
	for _, p := range site.Pages {
		fmt.Println(p.Path, "—", p.Title)
	}
	// Output:
	// index.html — thing
	// guide/start.html — Getting started
	// pkg/thing.html — thing
}

func ExampleCheck() {
	problems, _ := sitegen.Check("testdata/mod")
	for _, p := range problems {
		fmt.Println(p)
	}
	// Output:
	// example.com/thing/thing: symbol-doc Undocumented
	// example.com/thing/thing: example Undocumented
}

func ExampleCheckLinks() {
	out, _ := os.MkdirTemp("", "site")
	_, _ = sitegen.Build(sitegen.Config{Root: "testdata/mod", Out: out})
	problems, _ := sitegen.CheckLinks(out)
	fmt.Println(len(problems), "broken links")
	// Output: 0 broken links
}

func ExampleMust() {
	err := sitegen.Must([]sitegen.Problem{{Package: "p", Kind: "example", Name: "Run"}}, nil)
	fmt.Println(strings.Split(err.Error(), "\n")[0])
	// Output: sitegen: 1 problems
}

func ExampleDocHTML() {
	fmt.Println(sitegen.DocHTML("Add answers a plus b.\n\n# Why\n\nBecause."))
	// Output:
	// <p>Add answers a plus b.
	// <h3 id="hdr-Why">Why</h3>
	// <p>Because.
}

func ExampleMarkdown() {
	fmt.Print(sitegen.Markdown("## Start\n\nA *word*.\n"))
	// Output:
	// <h2 id="start">Start</h2>
	// <p>A <em>word</em>.</p>
}
