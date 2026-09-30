// Package sitegen builds a documentation site from a Go module's own
// source: the package comments, the exported API, the tested examples,
// and a guide of markdown pages. The site is the by-product of writing
// ordinary, documented Go; nothing is written twice.
//
// # What the site is made of
//
// Four sources, each optional except the source itself:
//
//   - README.md at the module root is the front page.
//   - Every package's doc comment (doc.go by convention) is its overview.
//   - Every exported identifier's doc comment is its reference entry,
//     rendered with the signature printed from the syntax tree.
//   - Every Example function in a _test.go file is shown with its code
//     and its Output block — go test proves the output, so the example
//     cannot rot.
//   - Markdown files under guide/ (or the directory named in Config) are
//     the narrative pages, ordered by their front matter.
//
// # The discipline it asks for
//
// The standard one, the one pkg.go.dev already expects: an exported name
// carries a comment that begins with the name; a package carries a
// comment; anything worth showing carries an example with output. Check
// reports what is missing so a test can refuse a release that forgets.
//
// # The theme
//
// Pages wear system.css from a CDN tag and nothing else; the generator
// ships no stylesheet and no script of its own. The shell is the engine's
// page grid: a header, a rail of sections, the page, an "On this page"
// aside, previous and next.
package sitegen
