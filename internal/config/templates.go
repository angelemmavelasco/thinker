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

	layouts, err := filepath.Glob(filepath.Join(templatesDir, "layouts", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	views, err := filepath.Glob(filepath.Join(templatesDir, "views", "*", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	for _, view := range views {
		lyCopy := make([]string, len(layouts))
		copy(lyCopy, layouts)
		files := append(lyCopy, view)

		tmpl := template.Must(
			template.New("base.html").Funcs(TemplateFuncs).ParseFiles(files...),
		)
		r.Add(filepath.Base(view), tmpl)
	}

	return r
}
