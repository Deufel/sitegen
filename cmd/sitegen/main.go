// Command sitegen builds a documentation site from a Go module:
//
//	sitegen -root . -out docs -title migrate -repo https://github.com/Deufel/migrate
//	sitegen -root . -check            # report what the discipline lacks, exit 1 if any
//	sitegen -root ../cobra -out /tmp/cobra-site -title cobra -serve :8080   # build, then serve it
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/Deufel/sitegen"
)

func main() {
	var cfg sitegen.Config
	check := flag.Bool("check", false, "report undocumented packages and symbols and functions without examples")
	serve := flag.String("serve", "", "after building, serve the site at this address (:8080) until interrupted")
	flag.StringVar(&cfg.Root, "root", ".", "the module's directory")
	flag.StringVar(&cfg.Out, "out", "docs", "the site's directory")
	flag.StringVar(&cfg.Title, "title", "", "the site's name (the module's last path element when empty)")
	flag.StringVar(&cfg.Tagline, "tagline", "", "a few words under the title")
	flag.StringVar(&cfg.Repo, "repo", "", "the repository URL (every symbol then links to its source line)")
	flag.StringVar(&cfg.Ref, "ref", "", "the branch, tag or commit the source links point at (HEAD when empty)")
	flag.StringVar(&cfg.Guide, "guide", "guide", "the markdown pages' directory, relative to root")
	flag.StringVar(&cfg.Theme, "theme", "", "the engine stylesheet's URL (system.css at a tag when empty)")
	flag.Parse()
	if *check {
		problems, err := sitegen.Check(cfg.Root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		for _, p := range problems {
			fmt.Println(p)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
		return
	}
	site, err := sitegen.Build(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := sitegen.Must(sitegen.CheckLinks(cfg.Out)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("sitegen: %d pages, %d packages → %s\n", len(site.Pages), len(site.Packages), cfg.Out)
	if *serve != "" {
		fmt.Printf("sitegen: serving %s at http://localhost%s/\n", cfg.Out, *serve)
		if err := http.ListenAndServe(*serve, http.FileServer(http.Dir(cfg.Out))); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
}
