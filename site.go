package sitegen

import (
	"bytes"
	"fmt"
	htmlesc "html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// Config names the module and where its site goes.
type Config struct {
	Root    string // the module's directory (holds go.mod)
	Out     string // the site's directory (created; index.html and the pages)
	Title   string // the site's name; the module's last path element when empty
	Tagline string // a few words under the title
	Repo    string // the repository URL, linked from the header
	Guide   string // the markdown pages' directory relative to Root; "guide" when empty; absent is fine
	// Theme is the engine stylesheet's URL; the pinned system.css tag when
	// empty. Highlight is its syntax companion ("" = the tag's).
	Theme, Highlight, HighlightJS string
}

// DefaultTheme — the engine at a tag; a site pins one and moves on purpose.
const (
	DefaultTheme       = "https://cdn.jsdelivr.net/gh/Deufel/system-css@v0.6.0/static/system.css"
	DefaultHighlight   = "https://cdn.jsdelivr.net/gh/Deufel/system-css@v0.6.0/static/highlight.css"
	DefaultHighlightJS = "https://cdn.jsdelivr.net/gh/Deufel/system-css@v0.6.0/static/highlight.js"
)

// Page is one rendered page of the site.
type Page struct {
	Path    string // relative to Out ("index.html", "guide/start.html", "pkg/migrate.html")
	Title   string
	Section string // "" (home) · "guide" · "pkg"
	Order   int
	Body    string // the rendered HTML inside the measure
}

// Site is what Build wrote.
type Site struct {
	Pages    []Page
	Packages []Package
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// Markdown renders GitHub-flavoured markdown to HTML with heading ids.
func Markdown(src string) string {
	var buf bytes.Buffer
	_ = md.Convert([]byte(src), &buf)
	return buf.String()
}

// Build reads the module and writes the site. It answers the pages it
// wrote, so a test can look at them without reading the disk again.
func Build(cfg Config) (Site, error) {
	if cfg.Root == "" || cfg.Out == "" {
		return Site{}, fmt.Errorf("sitegen: Root and Out are required")
	}
	if cfg.Guide == "" {
		cfg.Guide = "guide"
	}
	if cfg.Theme == "" {
		cfg.Theme = DefaultTheme
	}
	if cfg.Highlight == "" {
		cfg.Highlight = DefaultHighlight
	}
	if cfg.HighlightJS == "" {
		cfg.HighlightJS = DefaultHighlightJS
	}
	pkgs, err := Packages(cfg.Root)
	if err != nil {
		return Site{}, err
	}
	if cfg.Title == "" {
		if mp, err := modulePath(cfg.Root); err == nil {
			cfg.Title = mp[strings.LastIndex(mp, "/")+1:]
		}
	}
	var pages []Page
	// the front page: README.md, else the first package's overview
	if b, err := os.ReadFile(filepath.Join(cfg.Root, "README.md")); err == nil {
		pages = append(pages, Page{Path: "index.html", Title: cfg.Title, Body: repoLinks(Markdown(string(b)), cfg.Repo)})
	} else if len(pkgs) > 0 {
		pages = append(pages, Page{Path: "index.html", Title: cfg.Title, Body: DocHTML(pkgs[0].Doc)})
	} else {
		pages = append(pages, Page{Path: "index.html", Title: cfg.Title, Body: "<p>" + htmlesc.EscapeString(cfg.Title) + "</p>"})
	}
	guide, err := guidePages(filepath.Join(cfg.Root, cfg.Guide))
	if err != nil {
		return Site{}, err
	}
	pages = append(pages, guide...)
	for _, p := range pkgs {
		pages = append(pages, packagePage(p))
	}
	if err := os.MkdirAll(cfg.Out, 0o755); err != nil {
		return Site{}, err
	}
	for i := range pages {
		out := filepath.Join(cfg.Out, filepath.FromSlash(pages[i].Path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return Site{}, err
		}
		if err := os.WriteFile(out, []byte(shell(cfg, pages, &pages[i])), 0o644); err != nil {
			return Site{}, err
		}
	}
	return Site{Pages: pages, Packages: pkgs}, nil
}

var relHref = regexp.MustCompile(`href="(\./|/)?([A-Za-z0-9_][^":#?]*)"`)

// repoLinks — a README links to files of the repository (LICENSE,
// CONTRIBUTING.md, /middleware); on the site those point at the
// repository's own view of them, so the link check stays honest.
func repoLinks(body, repo string) string {
	if repo == "" {
		return body
	}
	return relHref.ReplaceAllStringFunc(body, func(m string) string {
		g := relHref.FindStringSubmatch(m)
		if strings.HasSuffix(g[2], ".html") {
			return m
		}
		return `href="` + strings.TrimRight(repo, "/") + `/blob/HEAD/` + g[2] + `"`
	})
}

var frontMatter = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)

// guidePages — the markdown files of the guide directory: front matter
// gives title and order; the file's first heading is the fallback title,
// the file name the fallback order.
func guidePages(dir string) ([]Page, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Page
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		src := string(b)
		pg := Page{Section: "guide", Path: "guide/" + strings.TrimSuffix(e.Name(), ".md") + ".html", Order: 1 << 20}
		if m := frontMatter.FindStringSubmatch(src); m != nil {
			src = src[len(m[0]):]
			for _, line := range strings.Split(m[1], "\n") {
				k, v, _ := strings.Cut(line, ":")
				v = strings.TrimSpace(v)
				switch strings.TrimSpace(k) {
				case "title":
					pg.Title = v
				case "order":
					pg.Order, _ = strconv.Atoi(v)
				}
			}
		}
		if pg.Title == "" {
			for _, line := range strings.Split(src, "\n") {
				if strings.HasPrefix(line, "# ") {
					pg.Title = strings.TrimSpace(line[2:])
					break
				}
			}
		}
		if pg.Title == "" {
			pg.Title = strings.TrimSuffix(e.Name(), ".md")
		}
		pg.Body = Markdown(src)
		out = append(out, pg)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// packagePage — one page per package: the overview, the API by kind, the
// examples. Every symbol is an h3 with the printed declaration and its
// comment; the aside lists the h2 sections.
func packagePage(p Package) Page {
	var b strings.Builder
	b.WriteString(`<p><code>import "` + htmlesc.EscapeString(p.ImportPath) + `"</code></p>`)
	b.WriteString(`<h2 id="overview">Overview</h2>` + DocHTML(p.Doc))
	sym := func(s Symbol) {
		b.WriteString(`<h3 id="` + htmlesc.EscapeString(anchor(s.Name)) + `">` + htmlesc.EscapeString(s.Name) + `</h3>`)
		b.WriteString(`<pre><code class="go">` + htmlesc.EscapeString(s.Sig) + `</code></pre>`)
		b.WriteString(DocHTML(s.Doc))
	}
	if len(p.Consts) > 0 {
		b.WriteString(`<h2 id="constants">Constants</h2>`)
		for _, s := range p.Consts {
			sym(s)
		}
	}
	if len(p.Vars) > 0 {
		b.WriteString(`<h2 id="variables">Variables</h2>`)
		for _, s := range p.Vars {
			sym(s)
		}
	}
	if len(p.Funcs) > 0 {
		b.WriteString(`<h2 id="functions">Functions</h2>`)
		for _, s := range p.Funcs {
			sym(s)
		}
	}
	if len(p.Types) > 0 {
		b.WriteString(`<h2 id="types">Types</h2>`)
		for _, t := range p.Types {
			sym(t.Symbol)
			for _, f := range t.Funcs {
				sym(f)
			}
			for _, m := range t.Methods {
				m.Name = t.Name + "." + m.Name
				sym(m)
			}
		}
	}
	if len(p.Examples) > 0 {
		b.WriteString(`<h2 id="examples">Examples</h2>`)
		for _, ex := range p.Examples {
			title := ex.For
			if title == "" {
				title = p.Name
			}
			if ex.Suffix != "" {
				title += " — " + ex.Suffix
			}
			b.WriteString(`<h3 id="` + htmlesc.EscapeString(anchor(ex.Name)) + `">` + htmlesc.EscapeString(title) + `</h3>`)
			if ex.Doc != "" {
				b.WriteString(DocHTML(ex.Doc))
			}
			b.WriteString(`<pre><code class="go">` + htmlesc.EscapeString(ex.Code) + `</code></pre>`)
			if ex.Output != "" {
				b.WriteString(`<small>Output</small><pre><code>` + htmlesc.EscapeString(ex.Output) + `</code></pre>`)
			}
		}
	}
	path := "pkg/" + strings.ReplaceAll(strings.Trim(p.Dir, "/"), "/", "-")
	if p.Dir == "" {
		path = "pkg/" + p.Name // the root package by its name (chi, not v5)
	}
	// the root package by its name, a nested one by its directory (ast and
	// extension/ast are two packages), a command by its directory too
	title := p.Name
	if p.Dir != "" {
		title = filepath.ToSlash(p.Dir)
	}
	return Page{Path: path + ".html", Title: title, Section: "pkg", Body: b.String()}
}

func anchor(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, ".", "-"), " ", "-"))
}

var preBlock = regexp.MustCompile(`(?s)<pre[^>]*>.*?</pre>`)

// blocks — every <pre> gets a block wrapper: inside the measure's flex
// column a scroll container would shrink to one line; a plain column
// around it keeps its content height (the engine site's own recipe).
func blocks(body string) string {
	return preBlock.ReplaceAllStringFunc(body, func(m string) string {
		return `<div class="column" style="--gap: 0;">` + m + `</div>`
	})
}

var h2html = regexp.MustCompile(`<h2 id="([^"]+)">(.+?)</h2>`)
var tagRe = regexp.MustCompile(`<[^>]+>`)

// toc — the page's h2 list, when it has three or more.
func toc(body string) string {
	ms := h2html.FindAllStringSubmatch(body, -1)
	if len(ms) < 3 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<nav class="toc column" aria-label="On this page" style="--gap: 0; --type: -1;"><small class="rail-head">On this page</small>`)
	for _, m := range ms {
		sb.WriteString(`<a href="#` + m[1] + `">` + tagRe.ReplaceAllString(m[2], "") + `</a>`)
	}
	sb.WriteString(`</nav>`)
	return sb.String()
}

// href — the relative link from one page to another.
func href(from, to string) string {
	depth := strings.Count(from, "/")
	return strings.Repeat("../", depth) + to
}

func navItem(link, label string, current bool) string {
	aria := ""
	if current {
		aria = ` aria-current="page"`
	}
	return `<a class="nav-item" href="` + link + `"` + aria + `><span class="medium large">` + htmlesc.EscapeString(label) + `</span></a>`
}

// shell — the engine's page grid around one page: the header, the rail
// of sections (Home · Guide · Packages), the page's own rail when its
// section has more than one page, the measure, the aside, previous and
// next.
func shell(cfg Config, pages []Page, pg *Page) string {
	root := strings.Repeat("../", strings.Count(pg.Path, "/"))
	var siblings []*Page
	for i := range pages {
		if pages[i].Section == pg.Section && pg.Section != "" {
			siblings = append(siblings, &pages[i])
		}
	}
	first := func(section string) *Page {
		for i := range pages {
			if pages[i].Section == section {
				return &pages[i]
			}
		}
		return nil
	}
	var nav strings.Builder
	nav.WriteString(navItem(root+"index.html", "Home", pg.Section == ""))
	for _, s := range [][2]string{{"guide", "Guide"}, {"pkg", "Packages"}} {
		if f := first(s[0]); f != nil {
			nav.WriteString(navItem(href(pg.Path, f.Path), s[1], pg.Section == s[0]))
		}
	}
	var rail strings.Builder
	if len(siblings) > 1 {
		label := map[string]string{"guide": "Guide", "pkg": "Packages"}[pg.Section]
		rail.WriteString(`<nav class="pg-toolbar spread-column tablet desktop" aria-label="` + label + `"><div class="column" style="--gap: 0.1lh;"><small class="rail-head medium large">` + label + `</small>`)
		for _, p := range siblings {
			rail.WriteString(navItem(href(pg.Path, p.Path), p.Title, p == pg))
		}
		rail.WriteString(`</div></nav>`)
	}
	crumbs := `<nav class="crumbs" aria-label="Breadcrumb"><a href="` + root + `index.html">` + htmlesc.EscapeString(cfg.Title) + `</a>`
	if f := first(pg.Section); f != nil && pg.Section != "" {
		crumbs += `<a href="` + href(pg.Path, f.Path) + `">` + map[string]string{"guide": "Guide", "pkg": "Packages"}[pg.Section] + `</a>`
	}
	crumbs += `</nav>`
	aside := ""
	if t := toc(pg.Body); t != "" {
		aside = `<aside class="pg-aside desktop">` + t + `</aside>`
	}
	foot := ""
	for i, p := range siblings {
		if p != pg {
			continue
		}
		var prev, next string
		if i > 0 {
			prev = `<a href="` + href(pg.Path, siblings[i-1].Path) + `">← ` + htmlesc.EscapeString(siblings[i-1].Title) + `</a>`
		}
		if i+1 < len(siblings) {
			next = `<a href="` + href(pg.Path, siblings[i+1].Path) + `">` + htmlesc.EscapeString(siblings[i+1].Title) + ` →</a>`
		}
		if prev != "" || next != "" {
			foot = `<footer class="pg-main-footer spread" style="--type: -1;"><span>` + prev + `</span><span>` + next + `</span></footer>`
		}
	}
	repo := ""
	if cfg.Repo != "" {
		repo = `<a href="` + htmlesc.EscapeString(cfg.Repo) + `" style="--type: -1;">Source</a>`
	}
	tagline := ""
	if cfg.Tagline != "" {
		tagline = `<small style="--fg: -0.55;">` + htmlesc.EscapeString(cfg.Tagline) + `</small>`
	}
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"/>
<meta name="color-scheme" content="dark light"/>
<title>` + htmlesc.EscapeString(pg.Title) + ` — ` + htmlesc.EscapeString(cfg.Title) + `</title>
<link rel="stylesheet" href="` + cfg.Theme + `"/>
<link rel="stylesheet" href="` + cfg.Highlight + `"/>
<style>@layer project { :where(.pg-aside) > :where(.toc) { position: sticky; inset-block-start: 0; } }</style>
</head>
<body>
<div class="page">
	<header class="pg-header spread" style="--gap: 0.5em;">
		<a href="` + root + `index.html" class="row oneline" style="--gap: 0.5em;"><strong>` + htmlesc.EscapeString(cfg.Title) + `</strong>` + tagline + `</a>
		<span class="row oneline" style="--gap: 0.5em;">` + repo + `</span>
	</header>
	<nav class="pg-navigation spread-column tablet desktop" aria-label="Sections">
		<div class="column">` + nav.String() + `</div>
	</nav>
	<header class="pg-main-header column">
		` + crumbs + `
		<div class="spread"><h1>` + htmlesc.EscapeString(pg.Title) + `</h1></div>
	</header>
	` + rail.String() + `
	<main class="pg-main column owns-scroll">
		<section class="column measure" style="--measure: 3; --gap: 1lh;">
` + blocks(pg.Body) + `
		</section>
	</main>
	` + aside + foot + `
</div>
<script src="` + cfg.HighlightJS + `"></script>
</body>
</html>
`
}
