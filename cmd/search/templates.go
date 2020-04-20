package main

import (
	"html/template"
	"path/filepath"
	"time"

	"github.com/tullo/search/internal/forms"
	"github.com/tullo/search/internal/product"
	"github.com/tullo/search/internal/user"
)

type templateData struct {
	CSRFToken       string
	CurrentYear     int
	Flash           string
	Form            *forms.Form
	Path            string
	IsAuthenticated bool
	Products        []product.Product
	Product         *product.Product
	User            *user.User
	Version         string
}

func humanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	// Convert the time to UTC before formatting it.
	return t.UTC().Format("02 Jan 2006 at 15:04")
}

func shortID(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:8]
}

func incr(idx int) int {
	return idx + 1
}

var functions = template.FuncMap{
	"humanDate": humanDate,
	"shortID":   shortID,
	"incr":      incr,
}

func newTemplateCache(dir string) (map[string]*template.Template, error) {

	cache := map[string]*template.Template{}

	// slice of filepaths with the extension '.page.tmpl'
	pages, err := filepath.Glob(filepath.Join(dir, "*.page.tmpl"))
	if err != nil {
		return nil, err
	}

	for _, page := range pages {

		// extract the file name
		name := filepath.Base(page)

		// parse the page template file into a template set
		ts, err := template.New(name).Funcs(functions).ParseFiles(page)
		if err != nil {
			return nil, err
		}

		// add any 'layout' templates to the template set
		ts, err = ts.ParseGlob(filepath.Join(dir, "*.layout.tmpl"))
		if err != nil {
			return nil, err
		}

		// add any 'partial' templates to the template set
		ts, err = ts.ParseGlob(filepath.Join(dir, "*.partial.tmpl"))
		if err != nil {
			return nil, err
		}

		// add the template set to the cache
		cache[name] = ts
	}

	return cache, nil
}
