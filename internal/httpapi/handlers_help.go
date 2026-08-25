package httpapi

import "net/http"

// Help pages are available to any authenticated user — they're generic
// documentation, not a permission-gated capability.

func (h *handlers) handleHelpIndex(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(h.help))
	for _, s := range h.help {
		out = append(out, map[string]any{"slug": s.Slug, "title": s.Title})
	}
	writeJSON(w, http.StatusOK, out, nil)
}

func (h *handlers) handleHelpPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	for _, s := range h.help {
		if s.Slug == slug {
			writeJSON(w, http.StatusOK, map[string]any{"slug": s.Slug, "title": s.Title, "html": s.HTML}, nil)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "not_found", "Help page not found")
}
