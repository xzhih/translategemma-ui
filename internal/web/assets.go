package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:generate ../../tools/sync_webui_dist.sh
//go:embed frontend/**
var webAssets embed.FS

func loadIndexHTML() ([]byte, error) {
	return webAssets.ReadFile("frontend/index.html")
}

func newStaticHandler() (http.Handler, error) {
	staticFS, err := fs.Sub(webAssets, "frontend/assets")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(staticFS)), nil
}
