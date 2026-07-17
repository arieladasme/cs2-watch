package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// The SvelteKit static build is embedded into the binary.
// Run `pnpm build` in web/ before `go build`.
//
//go:embed all:web/build
var webFS embed.FS

func registerWeb(mux *http.ServeMux) {
	sub, err := fs.Sub(webFS, "web/build")
	if err != nil {
		panic(err) // impossible: path is compile-time constant
	}
	mux.Handle("/", http.FileServerFS(sub))
}
