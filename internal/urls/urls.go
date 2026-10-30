package urls

import (
	"fmt"
)

// Routes is a centralized urls catalog to be maped automatically from templates
var RouteNames = map[string]string{
	"landing": "/",

	"api:users:list": "/api/users",

	"users:list": "/web/users",
	"users:new":  "/web/users/new",

	"ideas:list": "/web/ideas/list",
	"ideas:new":  "/web/ideas/new",
}

// Get resolves registered url path for gin
func Get(name string) string {
	if path, ok := RouteNames[name]; ok {
		return path
	}
	panic(fmt.Sprintf("#url-path-not-found: %s", name))
}

// URLFor resolves url path for templates with optional args
func URLFor(name string, args ...any) string {
	pattern, ok := RouteNames[name]
	if !ok {
		return fmt.Sprintf("#url-path-not-found: %s", name)
	}

	if len(args) == 0 {
		return pattern
	}

	return fmt.Sprintf(pattern, args...)
}
