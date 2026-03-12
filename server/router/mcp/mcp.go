package mcp

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

type MCPService struct {
	store         *store.Store
	authenticator *auth.Authenticator
}

func NewMCPService(store *store.Store, secret string) *MCPService {
	return &MCPService{
		store:         store,
		authenticator: auth.NewAuthenticator(store, secret),
	}
}

func (s *MCPService) RegisterRoutes(echoServer *echo.Echo) {
	mcpSrv := mcpserver.NewMCPServer("Memos", "1.0.0",
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithInstructions(
			"Memos is a personal knowledge management system with two content types:\n\n"+
				"## Memo\n"+
				"Free-form notes in Markdown for thoughts, ideas, links, or any content.\n"+
				"Supports #tag syntax, visibility control (PRIVATE/PROTECTED/PUBLIC), pinning, and archiving.\n\n"+
				"## Daily Log\n"+
				"A structured daily record inspired by Unix .plan files. Each user has exactly one log per day.\n"+
				"Content uses line prefixes: `* ` (completed work), `+ ` (planned/to-do), `- ` (notes), `? ` (open questions).\n"+
				"Only today's log can be edited (36-hour window); past logs are immutable. Always PROTECTED visibility.\n\n"+
				"Example daily log:\n"+
				"```\n"+
				"* shipped auth module to staging\n"+
				"* fixed pagination bug in list API\n"+
				"+ write unit tests for auth\n"+
				"- team decided to use PostgreSQL\n"+
				"? should we add rate limiting before launch?\n"+
				"```\n\n"+
				"## When to use which\n"+
				"- Recording daily progress or standup notes → memos_save_daily_log\n"+
				"- Capturing ideas, references, or free-form notes → memos_create_memo\n"+
				"- Searching past content → memos_search_memos\n\n"+
				"## Typical daily log workflow\n"+
				"1. Get today's log with memos_get_daily_log to see existing content\n"+
				"2. Append new lines and save with memos_save_daily_log (this replaces the full content)\n\n"+
				"## Timezone\n"+
				"Daily log tools accept an optional `timezone` parameter (e.g. 'Asia/Shanghai', '+08:00').\n"+
				"It defaults to +08:00. Pass the correct timezone to ensure date boundaries match the user's local calendar day.\n",
		),
	)
	s.registerMemoTools(mcpSrv)
	s.registerTagTools(mcpSrv)
	s.registerDailyLogTools(mcpSrv)
	s.registerPrompts(mcpSrv)

	httpHandler := mcpserver.NewStreamableHTTPServer(mcpSrv)

	mcpGroup := echoServer.Group("")
	mcpGroup.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))
	mcpGroup.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader != "" {
				result := s.authenticator.Authenticate(c.Request().Context(), authHeader)
				if result == nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"message": "invalid or expired token"})
				}
				ctx := auth.ApplyToContext(c.Request().Context(), result)
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	})
	mcpGroup.Any("/mcp", echo.WrapHandler(httpHandler))
}
