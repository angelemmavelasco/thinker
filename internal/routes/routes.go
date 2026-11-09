package routes

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"thinker/internal/auth"
	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/handlers"
	"thinker/internal/middleware"
	"thinker/internal/services"
	"thinker/internal/urls"
)

func SetUpRouter(
	queries *database.Queries,
	llm *services.LLMService,
	embedding *services.EmbeddingService,
	baseCfg ...*config.BaseConfig,
) *gin.Engine {
	var cfg *config.BaseConfig
	if len(baseCfg) > 0 && baseCfg[0] != nil {
		cfg = baseCfg[0]
	} else {
		loaded, err := config.LoadConfig()
		if err != nil {
			ephemeral := make([]byte, 32)
			for i := range ephemeral {
				ephemeral[i] = byte(i + 1)
			}
			cfg = &config.BaseConfig{
				Debug:         true,
				AppBaseURL:    "http://127.0.0.1:8080",
				JWTSecret:     ephemeral,
				SecureCookies: false,
			}
		} else {
			cfg = loaded
		}
	}

	jwtMgr, err := auth.NewJWTManager(cfg.JWTSecret, auth.DefaultAccessTTL)
	if err != nil {
		log.Fatalf("Error inicializando gestor JWT: %v", err)
	}

	mailer := services.NewMailerService(cfg)
	googleOAuth := services.NewGoogleOAuthService(cfg)
	authService := services.NewAuthService(queries, jwtMgr, mailer, googleOAuth)

	router := gin.Default()
	router.Use(middleware.SecurityHeadersMiddleware())
	router.Use(middleware.CSRFMiddleware(cfg.SecureCookies))
	router.Static("/static", "./static")
	router.HTMLRender = config.LoadTemplates("./templates/")

	authHandler := handlers.NewAuthHandler(authService, cfg.SecureCookies)
	ideaHandler := handlers.NewIdeaHandler(queries, llm, embedding)
	userHandler := handlers.NewUserHandler(queries)

	authRateLimiter := middleware.NewAuthRateLimiter(15, time.Minute, 5, 15*time.Minute)

	router.GET(urls.Get("landing"), func(c *gin.Context) {
		config.Render(c, http.StatusOK, "landing.html", gin.H{})
	})

	router.GET(urls.Get("auth:login"), authHandler.LoginView)
	router.POST(urls.Get("auth:login"), authRateLimiter.LimitEndpoint(), authHandler.Login)
	router.GET(urls.Get("auth:register"), authHandler.RegisterView)
	router.POST(urls.Get("auth:register"), authRateLimiter.LimitEndpoint(), authHandler.Register)
	router.GET(urls.Get("auth:verify_email"), authHandler.VerifyEmail)
	router.POST(urls.Get("auth:resend_verification"), authRateLimiter.LimitEndpoint(), authHandler.ResendVerification)
	router.GET(urls.Get("auth:forgot_password"), authHandler.ForgotPasswordView)
	router.POST(urls.Get("auth:forgot_password"), authRateLimiter.LimitEndpoint(), authHandler.ForgotPassword)
	router.GET(urls.Get("auth:reset_password"), authHandler.ResetPasswordView)
	router.POST(urls.Get("auth:reset_password"), authRateLimiter.LimitEndpoint(), authHandler.ResetPassword)
	router.POST(urls.Get("auth:refresh"), authHandler.RefreshToken)
	router.POST(urls.Get("auth:logout"), authHandler.Logout)
	router.POST(urls.Get("auth:logout_all"), authHandler.LogoutAll)
	router.GET(urls.Get("auth:google:login"), authHandler.GoogleLogin)
	router.GET(urls.Get("auth:google:callback"), authHandler.GoogleCallback)

	protected := router.Group("/")
	protected.Use(middleware.RequireAuthMiddleware(authService, cfg.SecureCookies))
	{
		protected.GET(urls.Get("api:users:list"), userHandler.ListUsers)
		protected.GET(urls.Get("api:network:graph"), ideaHandler.NetworkGraphJSON)
		protected.GET(urls.Get("network:view"), ideaHandler.NetworkView)
		protected.GET(urls.Get("ideas:list"), ideaHandler.ListIdeas)
		protected.GET(urls.Get("ideas:new"), ideaHandler.NewIdea)
		protected.POST(urls.Get("ideas:new"), ideaHandler.CreateIdea)
		protected.GET(urls.Get("ideas:detail:route"), ideaHandler.IdeaDetail)
		protected.GET(urls.Get("ideas:edit:route"), ideaHandler.EditIdeaView)
		protected.POST(urls.Get("ideas:edit:route"), ideaHandler.UpdateIdea)
		protected.POST(urls.Get("ideas:reanalyze:route"), ideaHandler.ReanalyzeIdea)
		protected.POST(urls.Get("ideas:delete:route"), ideaHandler.DeleteIdeaFragment)
		protected.POST(urls.Get("ideas:delete_group:route"), ideaHandler.DeleteIdeaGroup)

		protected.GET(urls.Get("users:list"), userHandler.ListUsersView)
		protected.GET(urls.Get("users:new"), userHandler.NewUser)
		protected.POST(urls.Get("users:new"), userHandler.CreateUser)
	}

	return router
}
