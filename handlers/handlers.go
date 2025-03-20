// package handlers contains the handler functions for our web service.
package handlers

import (
	"log"
	"net/http"

	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/frontend/templates"
)

// IsHtmxRequest checks if the request is coming from HTMX via AJAX
func IsHtmxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// RenderFullPageIfNotHtmx checks if the request is from HTMX.
// If not, it renders the full page template and returns true.
// If it is from HTMX, it returns false and the caller should continue with the handler.
func RenderFullPageIfNotHtmx(w http.ResponseWriter, r *http.Request, path string) bool {
	if IsHtmxRequest(r) {
		return false
	}

	// Not an HTMX request, render the full main template
	// but customize the initial hx-get to load the requested page
	w.Header().Set("Cache-Control", config.CacheControl)

	// Create a struct to pass to the template
	type Data struct {
		Path string
	}

	data := Data{
		Path: path,
	}

	if err := templates.Main.Execute(w, data); err != nil {
		log.Printf("Error executing main template with page %s: %v", path, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	return true
}
