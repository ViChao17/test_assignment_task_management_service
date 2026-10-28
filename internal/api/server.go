package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"taskmanager/docs"
	"taskmanager/internal/auth"
	"taskmanager/internal/cache"
	"taskmanager/internal/service"
	"taskmanager/internal/store"
)

type Server struct {
	auth   *service.AuthService
	teams  *service.TeamService
	tasks  *service.TaskService
	stats  *service.StatsService
	tokens *auth.TokenManager
	store  *store.Store
	cache  cache.TaskCache
	logger *slog.Logger
	mux    *http.ServeMux
}

func NewServer(
	authService *service.AuthService,
	teamService *service.TeamService,
	taskService *service.TaskService,
	statsService *service.StatsService,
	tokens *auth.TokenManager,
	storeInstance *store.Store,
	taskCache cache.TaskCache,
	logger *slog.Logger,
) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		auth:   authService,
		teams:  teamService,
		tasks:  taskService,
		stats:  statsService,
		tokens: tokens,
		store:  storeInstance,
		cache:  taskCache,
		logger: logger,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux
	handler = withSecurityHeaders(handler)
	handler = withLogging(s.logger)(handler)
	handler = withRecovery(s.logger)(handler)
	handler = withRequestID(handler)
	return handler
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	s.mux.HandleFunc("GET /swagger.yaml", s.handleOpenAPI)
	s.mux.HandleFunc("GET /docs", s.handleDocs)

	s.mux.HandleFunc("POST /api/v1/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/v1/login", s.handleLogin)

	s.mux.Handle("POST /api/v1/teams", s.requireAuth(s.handleCreateTeam))
	s.mux.Handle("GET /api/v1/teams", s.requireAuth(s.handleListTeams))
	s.mux.Handle("GET /api/v1/teams/{id}/members", s.requireAuth(s.handleListMembers))
	s.mux.Handle("POST /api/v1/teams/{id}/invite", s.requireAuth(s.handleInvite))
	s.mux.Handle("PATCH /api/v1/teams/{id}/members/{user_id}", s.requireAuth(s.handleUpdateMemberRole))
	s.mux.Handle("GET /api/v1/teams/{id}/stats", s.requireAuth(s.handleStats))

	s.mux.Handle("POST /api/v1/tasks", s.requireAuth(s.handleCreateTask))
	s.mux.Handle("GET /api/v1/tasks", s.requireAuth(s.handleListTasks))
	s.mux.Handle("GET /api/v1/tasks/{id}", s.requireAuth(s.handleGetTask))
	s.mux.Handle("PUT /api/v1/tasks/{id}", s.requireAuth(s.handleUpdateTask))
	s.mux.Handle("GET /api/v1/tasks/{id}/history", s.requireAuth(s.handleTaskHistory))
	s.mux.Handle("POST /api/v1/tasks/{id}/comments", s.requireAuth(s.handleCreateComment))
	s.mux.Handle("GET /api/v1/tasks/{id}/comments", s.requireAuth(s.handleListComments))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "dependency": "mysql"})
		return
	}
	if err := s.cache.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "dependency": "redis"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(docs.OpenAPI)
}

func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Task Manager API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({ url: '/openapi.yaml', dom_id: '#swagger-ui', deepLinking: true });
  </script>
</body>
</html>
`
