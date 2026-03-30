package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/auth"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/database"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the HTTP server with MCP handling.
type Server struct {
	httpServer      *http.Server
	mcpServer       *mcp.Server
	db              database.Store
	dsn             string
	shutdownTimeout time.Duration
}

// Config holds server configuration.
type Config struct {
	Port            string
	TLSCert         string
	TLSKey          string
	ShutdownTimeout time.Duration
	DSN             string
}

// New creates a new server.
func New(cfg Config, db database.Store, mcpServer *mcp.Server, validator *auth.CognitoValidator) *Server {
	mux := http.NewServeMux()

	// Health endpoint - no auth (MCP-04)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		if err := db.PingDedicated(r.Context(), cfg.DSN); err != nil {
			status = "degraded"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	})

	// MCP endpoint - with auth (AUTH-01)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, nil)
	authMiddleware := auth.Middleware(validator)
	mux.Handle("POST /mcp", authMiddleware(mcpHandler))

	// Wrap with security headers and panic recovery
	handler := securityHeaders(panicRecovery(mux))

	return &Server{
		httpServer: &http.Server{
			Addr:    ":" + cfg.Port,
			Handler: handler,
		},
		mcpServer:       mcpServer,
		db:              db,
		dsn:             cfg.DSN,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

// Start begins serving.
func (s *Server) Start() error {
	slog.Info("server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// StartTLS begins serving with TLS.
func (s *Server) StartTLS(certFile, keyFile string) error {
	slog.Info("server starting with TLS", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServeTLS(certFile, keyFile)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// securityHeaders adds required security headers (SEC-14).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// panicRecovery catches panics and returns 500 (ERR-18).
func panicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				slog.Error("panic recovered",
					"error", fmt.Sprintf("%v", err),
					"stack", string(stack),
				)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
