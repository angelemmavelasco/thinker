package config

import "github.com/gin-gonic/gin"

func GlobalContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user", "00000000-0000-0000-0000-000000000001")
		c.Next()
	}
}
