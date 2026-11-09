package urls

import (
	"fmt"
)

var RouteNames = map[string]string{
	"landing": "/",

	"auth:login":               "/auth/login",
	"auth:register":            "/auth/register",
	"auth:verify_email":        "/auth/verify-email",
	"auth:resend_verification": "/auth/resend-verification",
	"auth:forgot_password":     "/auth/forgot-password",
	"auth:reset_password":      "/auth/reset-password",
	"auth:refresh":             "/auth/refresh",
	"auth:logout":              "/auth/logout",
	"auth:logout_all":          "/auth/logout-all",
	"auth:google:login":        "/auth/google/login",
	"auth:google:callback":     "/auth/google/callback",

	"api:users:list": "/api/users",

	"users:list": "/web/users",
	"users:new":  "/web/users/new",

	"ideas:list":               "/web/ideas/list",
	"ideas:new":                "/web/ideas/new",
	"ideas:detail:route":       "/web/ideas/:id",
	"ideas:detail":             "/web/ideas/%v",
	"ideas:edit:route":         "/web/ideas/:id/edit",
	"ideas:edit":               "/web/ideas/%v/edit",
	"ideas:reanalyze:route":    "/web/ideas/:id/reanalyze",
	"ideas:reanalyze":          "/web/ideas/%v/reanalyze",
	"ideas:delete:route":       "/web/ideas/:id/delete",
	"ideas:delete":             "/web/ideas/%v/delete",
	"ideas:delete_group:route": "/web/ideas/:id/delete-group",
	"ideas:delete_group":       "/web/ideas/%v/delete-group",

	"network:view":      "/web/network",
	"api:network:graph": "/api/network/graph",
}

func Get(name string) string {
	if path, ok := RouteNames[name]; ok {
		return path
	}
	panic(fmt.Sprintf("#url-path-not-found: %s", name))
}

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
