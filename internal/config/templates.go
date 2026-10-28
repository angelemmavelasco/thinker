package config

import (
	"html/template"
	"path/filepath"
	"thinker/internal/urls"

	"github.com/gin-contrib/multitemplate"
)

var TemplateFuncs = template.FuncMap{
	"url": urls.URLFor,
}

func LoadTemplates(templatesDir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	includes, err := filepath.Glob(filepath.Join(templatesDir, "includes", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	layouts, err := filepath.Glob(filepath.Join(templatesDir, "layouts", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	views, err := filepath.Glob(filepath.Join(templatesDir, "views", "*", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	for _, view := range views {
		files := append([]string{}, layouts...)
		files = append(files, includes...)
		files = append(files, view)

		tmpl := template.Must(
			template.New("base.html").Funcs(TemplateFuncs).ParseFiles(files...),
		)
		r.Add(filepath.Base(view), tmpl)
	}

	pages, err := filepath.Glob(filepath.Join(templatesDir, "pages", "*", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	for _, page := range pages {
		name := filepath.Base(page)
		files := append([]string{}, includes...)
		files = append(files, page)

		tmpl := template.Must(
			template.New(name).Funcs(TemplateFuncs).ParseFiles(files...),
		)
		r.Add(name, tmpl)
	}

	return r
}
