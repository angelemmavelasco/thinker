package config

import (
	"path/filepath"

	"github.com/gin-contrib/multitemplate"
)

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
		r.AddFromFiles(filepath.Base(view), files...)
	}

	return r
}
