# sitegen

A documentation site from a Go module's own source: the package comments,
the exported API with its signatures, the tested examples with their
output, a guide of markdown pages. Write ordinary documented Go and the
site is the by-product.

```
go run github.com/Deufel/sitegen/cmd/sitegen -root . -out docs -title migrate -repo https://github.com/Deufel/migrate
go run github.com/Deufel/sitegen/cmd/sitegen -root . -check   # what the discipline lacks
```

The discipline is the standard one: an exported name carries a comment
that begins with the name, a package carries a comment, a function
carries an example with output. `Check` reports the gaps; a test refuses
a release that forgets. Pages wear system.css from a CDN tag and nothing
else.
