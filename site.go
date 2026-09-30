package sitegen

import (
	"bytes"
	"encoding/json"
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
	Repo    string // the repository URL, linked from the header; with Ref, every symbol links to its source line
	Ref     string // the branch, tag or commit the source links point at; "HEAD" when empty
	Guide   string // the markdown pages' directory relative to Root; "guide" when empty; absent is fine
	// Theme is the engine stylesheet's URL; the pinned system.css tag when
	// empty. Highlight is its syntax companion ("" = the tag's). Datastar
	// drives the search dialog ("" = the free bundle at a tag).
	Theme, Highlight, HighlightJS, Datastar string
}

// DefaultTheme — the engine at a tag; a site pins one and moves on purpose.
const (
	DefaultTheme       = "https://cdn.jsdelivr.net/gh/Deufel/system-css@v0.6.0/static/system.css"
	DefaultHighlight   = "https://cdn.jsdelivr.net/gh/Deufel/system-css@v0.6.0/static/highlight.css"
	DefaultHighlightJS = "https://cdn.jsdelivr.net/gh/Deufel/system-css@v0.6.0/static/highlight.js"
	DefaultDatastar    = "https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.4/bundles/datastar.js"
)

// Page is one rendered page of the site.
type Page struct {
	Path    string // relative to Out ("index.html", "guide/start.html", "pkg/migrate.html")
	Title   string
	Section string // "" (home) · "guide" · "pkg"
	Order   int
	Body    string  // the rendered HTML inside the measure
	Index   string  // the page's own index for the n3 rail: a package's symbols by kind, a markdown page's headings
	Entries []Entry // what the search finds on this page
}

// Entry is one thing the site-wide search can land on: a page, a symbol,
// an example, a heading.
type Entry struct {
	Title  string
	Kind   string // "page" · "const" · "var" · "func" · "type" · "method" · "example" · "heading"
	Anchor string // "" = the page itself
	Page   string // the page's path (filled by Build)
	Where  string // the page's title, shown beside a hit
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
	if cfg.Ref == "" {
		cfg.Ref = "HEAD"
	}
	if cfg.Datastar == "" {
		cfg.Datastar = DefaultDatastar
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
		body := repoLinks(Markdown(string(b)), cfg.Repo)
		pages = append(pages, Page{Path: "index.html", Title: cfg.Title, Body: body, Entries: headings(body)})
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
		pages = append(pages, packagePage(cfg, p))
	}
	if err := os.MkdirAll(cfg.Out, 0o755); err != nil {
		return Site{}, err
	}
	var entries []Entry
	for i := range pages {
		if pages[i].Index == "" {
			pages[i].Index = headingIndex(pages[i].Body)
		}
		entries = append(entries, Entry{Title: pages[i].Title, Kind: "page", Page: pages[i].Path, Where: pages[i].Title})
		for _, e := range pages[i].Entries {
			e.Page, e.Where = pages[i].Path, pages[i].Title
			entries = append(entries, e)
		}
	}
	for i := range pages {
		out := filepath.Join(cfg.Out, filepath.FromSlash(pages[i].Path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return Site{}, err
		}
		if err := os.WriteFile(out, []byte(shell(cfg, pages, entries, &pages[i])), 0o644); err != nil {
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
		pg.Entries = headings(pg.Body)
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
// examples. Every symbol is an h3 with the printed declaration, its
// comment and, when the repository is known, a link to the line it is
// written on; the aside lists the h2 sections.
func packagePage(cfg Config, p Package) Page {
	var b strings.Builder
	b.WriteString(`<p><code>import "` + htmlesc.EscapeString(p.ImportPath) + `"</code></p>`)
	b.WriteString(`<h2 id="overview">Overview</h2>` + DocHTML(p.Doc))
	source := func(file string, line int) string {
		if cfg.Repo == "" || file == "" {
			return ""
		}
		u := strings.TrimRight(cfg.Repo, "/") + "/blob/" + cfg.Ref + "/" + file + "#L" + strconv.Itoa(line)
		return `<a class="icon" href="` + htmlesc.EscapeString(u) + `" aria-label="Source" title="` + htmlesc.EscapeString(file+":"+strconv.Itoa(line)) + `" style="--type: -1; --fg: -0.55;">` + iconCode + `</a>`
	}
	head := func(id, title, src string) {
		b.WriteString(`<h3 id="` + htmlesc.EscapeString(id) + `" class="spread">` + htmlesc.EscapeString(title) + src + `</h3>`)
	}
	var ix strings.Builder
	ixHead := func(id, label string) {
		ix.WriteString(`<a class="nav-item" href="#` + id + `" style="margin-block-start: 0.5lh;"><span class="rail-head">` + label + `</span></a>`)
	}
	ixItem := func(id, label string) {
		ix.WriteString(`<a class="nav-item" href="#` + htmlesc.EscapeString(id) + `"><span>` + htmlesc.EscapeString(label) + `</span></a>`)
	}
	var entries []Entry
	kind := "func"
	sym := func(s Symbol) {
		head(anchor(s.Name), s.Name, source(s.File, s.Line))
		ixItem(anchor(s.Name), s.Name)
		entries = append(entries, Entry{Title: s.Name, Kind: kind, Anchor: anchor(s.Name)})
		b.WriteString(`<pre><code class="go">` + htmlesc.EscapeString(s.Sig) + `</code></pre>`)
		b.WriteString(DocHTML(s.Doc))
	}
	ixHead("overview", "Overview")
	if len(p.Consts) > 0 {
		b.WriteString(`<h2 id="constants">Constants</h2>`)
		ixHead("constants", "Constants")
		kind = "const"
		for _, s := range p.Consts {
			sym(s)
		}
	}
	if len(p.Vars) > 0 {
		b.WriteString(`<h2 id="variables">Variables</h2>`)
		ixHead("variables", "Variables")
		kind = "var"
		for _, s := range p.Vars {
			sym(s)
		}
	}
	if len(p.Funcs) > 0 {
		b.WriteString(`<h2 id="functions">Functions</h2>`)
		ixHead("functions", "Functions")
		kind = "func"
		for _, s := range p.Funcs {
			sym(s)
		}
	}
	if len(p.Types) > 0 {
		b.WriteString(`<h2 id="types">Types</h2>`)
		ixHead("types", "Types")
		for _, t := range p.Types {
			kind = "type"
			sym(t.Symbol)
			kind = "func"
			for _, f := range t.Funcs {
				sym(f)
			}
			kind = "method"
			for _, m := range t.Methods {
				m.Name = t.Name + "." + m.Name
				sym(m)
			}
		}
	}
	if len(p.Examples) > 0 {
		b.WriteString(`<h2 id="examples">Examples</h2>`)
		ixHead("examples", "Examples")
		for _, ex := range p.Examples {
			title := ex.For
			if title == "" {
				title = p.Name
			}
			if ex.Suffix != "" {
				title += " — " + ex.Suffix
			}
			head(anchor(ex.Name), title, source(ex.File, ex.Line))
			ixItem(anchor(ex.Name), title)
			entries = append(entries, Entry{Title: title, Kind: "example", Anchor: anchor(ex.Name)})
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
	return Page{Path: path + ".html", Title: title, Section: "pkg", Body: b.String(), Index: ix.String(), Entries: entries}
}

// searchText — an entry's haystack as a JS string literal (json), escaped
// for the attribute it sits in.
func searchText(e Entry) string {
	b, _ := json.Marshal(strings.ToLower(e.Title + " " + e.Kind + " " + e.Where))
	return htmlesc.EscapeString(string(b))
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

// headings — a markdown page's h2s as search entries.
func headings(body string) []Entry {
	var out []Entry
	for _, m := range h2html.FindAllStringSubmatch(body, -1) {
		out = append(out, Entry{Title: tagRe.ReplaceAllString(m[2], ""), Kind: "heading", Anchor: m[1]})
	}
	return out
}

// headingIndex — a markdown page's h2 list as its index, when it has two
// or more.
func headingIndex(body string) string {
	ms := h2html.FindAllStringSubmatch(body, -1)
	if len(ms) < 2 {
		return ""
	}
	var sb strings.Builder
	for _, m := range ms {
		sb.WriteString(`<a class="nav-item" href="#` + m[1] + `"><span>` + tagRe.ReplaceAllString(m[2], "") + `</span></a>`)
	}
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
	return `<a class="nav-item" href="` + link + `"` + aria + `><span>` + htmlesc.EscapeString(label) + `</span></a>`
}

// Lucide glyphs, drawn inline: the shell ships no icon file.
const (
	iconMenu   = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 5h16"/><path d="M4 12h16"/><path d="M4 19h16"/></svg>`
	iconSun    = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>`
	iconMoon   = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>`
	iconGitHub = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4"/><path d="M9 18c-4.51 2-5-2-7-2"/></svg>`
	iconSearch = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`
	iconCode   = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m16 18 6-6-6-6"/><path d="m8 6-6 6 6 6"/></svg>`
)

// shell — the engine's page grid around one page: the header (the menu
// on a phone, the title, the theme toggle, the source link), ONE
// navigation of every page — Home, the guide, the packages — as a rail
// on wide screens and a left drawer on a phone, the page's own index on
// the n3 rail (a package's symbols by kind, a markdown page's headings),
// the measure, previous and next.
func shell(cfg Config, pages []Page, entries []Entry, pg *Page) string {
	root := strings.Repeat("../", strings.Count(pg.Path, "/"))
	var siblings []*Page
	for i := range pages {
		if pages[i].Section == pg.Section && pg.Section != "" {
			siblings = append(siblings, &pages[i])
		}
	}
	// the one navigation, grouped: Home · Guide · Packages
	var nav strings.Builder
	nav.WriteString(`<div class="column" style="--gap: 0.1lh;">`)
	nav.WriteString(navItem(root+"index.html", "Home", pg.Section == ""))
	for _, s := range [][2]string{{"guide", "Guide"}, {"pkg", "Packages"}} {
		var group []Page
		for _, p := range pages {
			if p.Section == s[0] {
				group = append(group, p)
			}
		}
		if len(group) == 0 {
			continue
		}
		nav.WriteString(`<small class="rail-head medium large" style="margin-block-start: 0.5lh;">` + s[1] + `</small>`)
		for _, p := range group {
			nav.WriteString(navItem(href(pg.Path, p.Path), p.Title, p.Path == pg.Path))
		}
	}
	nav.WriteString(`</div>`)
	crumbs := `<nav class="crumbs" aria-label="Breadcrumb"><a href="` + root + `index.html">` + htmlesc.EscapeString(cfg.Title) + `</a>`
	if pg.Section != "" {
		crumbs += `<span>` + map[string]string{"guide": "Guide", "pkg": "Packages"}[pg.Section] + `</span>`
	}
	crumbs += `</nav>`
	// the n3 rail: the page's own index
	index := ""
	if pg.Index != "" {
		index = `<nav class="pg-toolbar spread-column tablet desktop" aria-label="On this page" style="--type: -1;"><div class="column" style="--gap: 0.1lh;">` + pg.Index + `</div></nav>`
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
		repo = `<a class="icon" href="` + htmlesc.EscapeString(cfg.Repo) + `" aria-label="Source on GitHub" title="Source on GitHub">` + iconGitHub + `</a>`
	}
	tagline := ""
	if cfg.Tagline != "" {
		tagline = `<small class="tablet desktop" style="--fg: -0.55;">` + htmlesc.EscapeString(cfg.Tagline) + `</small>`
	}
	// THE SEARCH (the ds_docs_demo pattern, on the engine): every entry of
	// the site pre-rendered in one dialog, shown by a Datastar expression
	// over the query, ordered by how well it matches — no server, no index
	// file, no client list rendering
	var hits strings.Builder
	for _, e := range entries {
		link := href(pg.Path, e.Page)
		if e.Anchor != "" {
			link += "#" + e.Anchor
		}
		hay := searchText(e)
		hits.WriteString(`<a class="nav-item" href="` + htmlesc.EscapeString(link) + `" data-show="hit(` + hay + `, $q)" data-style:order="-rank(` + hay + `, $q)" style="display: none;"><span>` + htmlesc.EscapeString(e.Title) + `</span><small style="--fg: -0.55; margin-inline-start: auto;">` + htmlesc.EscapeString(e.Kind+" · "+e.Where) + `</small></a>`)
	}
	search := `<button type="button" class="icon" aria-label="Search" title="Search (/)" onclick="openSearch()">` + iconSearch + `</button>`
	searchDialog := `<dialog id="site-search" class="modal glass" closedby="any" aria-label="Search" data-signals="{q: ''}" data-on:keydown__window="evt.key === '/' && !el.open && (evt.preventDefault(), openSearch())">
		<div class="column" style="--gap: 0.5lh;">
			<label class="search-box">` + iconSearch + `<input type="search" placeholder="Search…" data-bind:q autocomplete="off"/><kbd>esc</kbd></label>
			<div class="column scroll-y" data-show="$q.trim().length > 0" style="--gap: 0; max-block-size: 60vh; display: none;">` + hits.String() + `</div>
		</div>
	</dialog>`
	// the theme toggle: light ↔ dark on the root, remembered in storage
	// under the engine site's own key; the glyph shows what a tap gives
	theme := `<button type="button" class="icon" aria-label="Theme" title="Light or dark" onclick="toggleTheme()"><span class="to-dark">` + iconMoon + `</span><span class="to-light">` + iconSun + `</span></button>`
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"/>
<meta name="color-scheme" content="dark light"/>
<title>` + htmlesc.EscapeString(pg.Title) + ` — ` + htmlesc.EscapeString(cfg.Title) + `</title>
<link rel="stylesheet" href="` + cfg.Theme + `"/>
<link rel="stylesheet" href="` + cfg.Highlight + `"/>
<style>@layer project {
  :root:not([data-ui-theme="dark"]) .to-light, :root[data-ui-theme="dark"] .to-dark { display: none; }
  :where(h3.spread) > :where(a.icon) { --bg: 0; }
}</style>
<script>
// the stored theme before first paint (the interstitial frame is pre-CSS)
(function () { try { var t = localStorage.getItem('ui.theme'); if (t) document.documentElement.setAttribute('data-ui-theme', t) } catch (e) {} })();
function openSearch() { var d = document.getElementById('site-search'); d.showModal(); d.querySelector('input').focus() }
// the search: every term of the query somewhere in the entry's text; the
// rank counts terms at a word start, so "run" lists Run before Rerun
function terms(q) { return q.toLowerCase().trim().split(/\s+/).filter(Boolean) }
function hit(h, q) { var t = terms(q); return t.length > 0 && t.every(function (x) { return h.indexOf(x) >= 0 }) }
function rank(h, q) { return terms(q).reduce(function (n, x) { return n + (h.indexOf(x) === 0 || h.indexOf(' ' + x) >= 0 || h.indexOf('.' + x) >= 0 ? 2 : 1) }, 0) }
function toggleTheme() {
  var h = document.documentElement, dark = h.getAttribute('data-ui-theme') === 'dark' || (!h.getAttribute('data-ui-theme') && matchMedia('(prefers-color-scheme: dark)').matches);
  var t = dark ? 'light' : 'dark'; h.setAttribute('data-ui-theme', t); try { localStorage.setItem('ui.theme', t) } catch (e) {}
}
</script>
</head>
<body>
<div class="page">
	<header class="pg-header spread" style="--gap: 0.5em;">
		<span class="row oneline" style="--gap: 0.5em;">
			<button type="button" class="icon mobile" aria-label="Menu" title="Menu" onclick="document.getElementById('site-nav').showModal()">` + iconMenu + `</button>
			<a href="` + root + `index.html" class="row oneline" style="--gap: 0.5em;"><strong>` + htmlesc.EscapeString(cfg.Title) + `</strong>` + tagline + `</a>
		</span>
		<span class="row oneline" style="--gap: 0.25em;">` + search + theme + repo + `</span>
	</header>
	` + searchDialog + `
	<nav class="pg-navigation spread-column tablet desktop" aria-label="Pages">` + nav.String() + `</nav>
	<dialog id="site-nav" class="drawer left" closedby="any" aria-label="Pages">` + nav.String() + `</dialog>
	<header class="pg-main-header column">
		` + crumbs + `
		<div class="spread"><h1>` + htmlesc.EscapeString(pg.Title) + `</h1></div>
	</header>
	` + index + `
	<main class="pg-main column owns-scroll">
		<section class="column measure" style="--measure: 3; --gap: 1lh;">
` + blocks(pg.Body) + `
		</section>
	</main>
	` + foot + `
</div>
<script src="` + cfg.HighlightJS + `"></script>
<script type="module" src="` + cfg.Datastar + `"></script>
</body>
</html>
`
}
