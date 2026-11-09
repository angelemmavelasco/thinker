package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAuthRateLimiter_ByIPAndAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limiter := NewAuthRateLimiter(10, time.Minute, 3, 15*time.Minute)

	r := gin.New()
	r.POST("/auth/login", limiter.LimitEndpoint(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 1; i <= 4; i++ {
		form := url.Values{}
		form.Set("email", "TargetUser@Example.com")
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "127.0.0.1:54321"

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if i <= 3 && w.Code != http.StatusOK {
			t.Fatalf("attempt %d expected 200 OK, got %d", i, w.Code)
		}
		if i == 4 {
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("attempt 4 expected 429 Too Many Requests, got %d", w.Code)
			}
			if w.Header().Get("Retry-After") == "" {
				t.Fatalf("expected Retry-After header on 429 response")
			}
		}
	}
}
