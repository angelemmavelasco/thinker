package config

import (
	"html/template"
	"path/filepath"
	"sync"

	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"

	"thinker/internal/urls"
)

var TemplateFuncs = template.FuncMap{
	"url": urls.URLFor,
	"truncate": func(s string, max int) string {
		runes := []rune(s)
		if len(runes) <= max {
			return s
		}
		return string(runes[:max]) + "..."
	},
}

type reloadableHTMLRender struct {
	mu           sync.RWMutex
	templatesDir string
	inner        multitemplate.Renderer
	known        map[string]bool
}

func (rr *reloadableHTMLRender) Instance(name string, data any) render.Render {
	if gin.IsDebugging() {
		inner, known := buildRenderer(rr.templatesDir)
		rr.mu.Lock()
		rr.inner = inner
		rr.known = known
		rr.mu.Unlock()
		return inner.Instance(name, data)
	}

	rr.mu.RLock()
	exists := rr.known[name]
	inner := rr.inner
	rr.mu.RUnlock()

	if !exists {
		rr.mu.Lock()
		if !rr.known[name] {
			rr.inner, rr.known = buildRenderer(rr.templatesDir)
		}
		inner = rr.inner
		rr.mu.Unlock()
	}

	return inner.Instance(name, data)
}

func LoadTemplates(templatesDir string) render.HTMLRender {
	inner, known := buildRenderer(templatesDir)
	return &reloadableHTMLRender{
		templatesDir: templatesDir,
		inner:        inner,
		known:        known,
	}
}

func buildRenderer(templatesDir string) (multitemplate.Renderer, map[string]bool) {
	r := multitemplate.NewRenderer()
	known := make(map[string]bool)

	includes, err := filepath.Glob(filepath.Join(templatesDir, "includes", "*.html"))
	if err != nil {
		panic(err.Error())
	}

	layouts, err := filepath.Glob(filepath.Join(templatesDir, "layouts", "*.html"))
	if err != nil {
		panic(err.Error())
	}

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
		known[baseName] = true
		if slashName != baseName {
			r.Add(slashName, tmpl)
			known[slashName] = true
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
		known[baseName] = true
		if slashName != baseName {
			r.Add(slashName, tmpl)
			known[slashName] = true
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
		known[baseName] = true
		if slashName != baseName {
			r.Add(slashName, tmpl)
			known[slashName] = true
		}
	}

	return r, known
}

func Render(c *gin.Context, status int, templateName string, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	data["user"] = c.GetString("user")
	data["user_email"] = c.GetString("user_email")
	data["user_role"] = c.GetString("user_role")
	data["csrf_token"] = c.GetString("csrf_token")

	c.HTML(status, templateName, data)
}
