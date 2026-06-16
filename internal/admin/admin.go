package admin

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/configkits/mcp-gateway/internal/version"
)

//go:embed dashboard.html
var dashboardHTML string

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		page := strings.Replace(dashboardHTML, "__VERSION__", version.String, 1)
		w.Write([]byte(page))
	})
}
