package config

import (
	"html/template"
	"path/filepath"

	"github.com/gin-contrib/multitemplate"

	"thinker/internal/urls"
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

	//partials for templates/partials/*/*.html and templates/partials/*.html
	partials, err := filepath.Glob(filepath.Join(templatesDir, "partials", "*", "*.html"))
	if err != nil {
		panic(err.Error())
	}
	flatPartials, _ := filepath.Glob(filepath.Join(templatesDir, "partials", "*.html"))
	partials = append(partials, flatPartials...)

	views, err := filepath.Glob(filepath.Join(templatesDir, "views", "*", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	for _, view := range views {
		baseName := filepath.Base(view)
		relPath, _ := filepath.Rel(filepath.Join(templatesDir, "views"), view)
		slashName := filepath.ToSlash(relPath)

		files := append([]string{}, layouts...)
		files = append(files, includes...)
		files = append(files, partials...)
		files = append(files, view)

		tmpl := template.Must(
			template.New("base.html").Funcs(TemplateFuncs).ParseFiles(files...),
		)
		r.Add(baseName, tmpl)
		if slashName != baseName {
			r.Add(slashName, tmpl)
		}
	}

	pages, err := filepath.Glob(filepath.Join(templatesDir, "pages", "*", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	for _, page := range pages {
		baseName := filepath.Base(page)
		relPath, _ := filepath.Rel(filepath.Join(templatesDir, "pages"), page)
		slashName := filepath.ToSlash(relPath)

		files := append([]string{}, includes...)
		files = append(files, page)

		tmpl := template.Must(
			template.New(baseName).Funcs(TemplateFuncs).ParseFiles(files...),
		)
		r.Add(baseName, tmpl)
		if slashName != baseName {
			r.Add(slashName, tmpl)
		}
	}

	for _, partial := range partials {
		baseName := filepath.Base(partial)
		relPath, _ := filepath.Rel(filepath.Join(templatesDir, "partials"), partial)
		slashName := filepath.ToSlash(relPath)

		files := append([]string{}, includes...)
		files = append(files, partial)

		tmpl := template.Must(template.New(baseName).Funcs(TemplateFuncs).ParseFiles(files...))

		r.Add(baseName, tmpl)
		if slashName != baseName {
			r.Add(slashName, tmpl)
		}
	}

	return r
}
