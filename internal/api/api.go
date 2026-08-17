package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/3xiaoxiao3/mihomo-stack/internal/auth"
	"github.com/3xiaoxiao3/mihomo-stack/internal/state"
	"github.com/3xiaoxiao3/mihomo-stack/internal/updater"
)

type UpdateService interface {
	Update(context.Context, string) (state.Record, error)
	Restore(context.Context, string, string) (state.Record, error)
	ListBackups() ([]updater.Backup, error)
	Busy() bool
}

type HistoryStore interface {
	History() []state.Record
}

type MihomoClient interface {
	Health(context.Context) error
	Version(context.Context) (string, error)
}

type Options struct {
	Version          string
	WebDir           string
	DashboardURL     string
	ActiveConfigPath string
	DataDir          string
	SchedulerEnabled bool
	UpdateInterval   time.Duration
	SourceCount      int
}

type Server struct {
	options Options
	auth    *auth.Manager
	updates UpdateService
	history HistoryStore
	mihomo  MihomoClient
	logger  *slog.Logger
	handler http.Handler
}

func New(options Options, authManager *auth.Manager, updates UpdateService, history HistoryStore, mihomo MihomoClient, logger *slog.Logger) *Server {
	server := &Server{
		options: options,
		auth:    authManager,
		updates: updates,
		history: history,
		mihomo:  mihomo,
		logger:  logger,
	}
	server.handler = server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/healthz", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)

	protect := func(pattern string, handler http.HandlerFunc) {
		mux.Handle(pattern, s.auth.Require(handler, s.unauthorized))
	}
	protect("GET /api/v1/status", s.status)
	protect("GET /api/v1/history", s.getHistory)
	protect("POST /api/v1/updates", s.update)
	protect("GET /api/v1/backups", s.backups)
	protect("POST /api/v1/backups/{id}/restore", s.restore)
	protect("GET /api/v1/diagnostics", s.diagnostics)
	mux.HandleFunc("/", s.web)

	var handler http.Handler = mux
	handler = s.logging(handler)
	handler = requestIDs(handler)
	handler = securityHeaders(handler)
	handler = recoverPanics(handler, s.logger)
	return handler
}

func (s *Server) health(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.options.Version,
		"time":    time.Now().UTC(),
	})
}

func (s *Server) login(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
	defer request.Body.Close()
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(request.Body, &body); err != nil {
		s.writeError(response, request, http.StatusBadRequest, "invalid_request", "请求内容无效")
		return
	}
	if !s.auth.ValidateToken(body.Token) {
		s.writeError(response, request, http.StatusUnauthorized, "invalid_credentials", "管理员令牌无效")
		return
	}
	s.auth.SetSession(response, request)
	writeJSON(response, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) logout(response http.ResponseWriter, request *http.Request) {
	s.auth.ClearSession(response, request)
	writeJSON(response, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) status(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	mihomoVersion, err := s.mihomo.Version(ctx)
	online := err == nil
	if !online {
		mihomoVersion = ""
	}
	history := s.history.History()
	var lastUpdate *state.Record
	if len(history) > 0 {
		lastUpdate = &history[0]
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"guardian_version": s.options.Version,
		"mihomo": map[string]any{
			"online":  online,
			"version": mihomoVersion,
		},
		"update_busy":       s.updates.Busy(),
		"scheduler_enabled": s.options.SchedulerEnabled,
		"update_interval":   s.options.UpdateInterval.String(),
		"source_count":      s.options.SourceCount,
		"dashboard_url":     s.options.DashboardURL,
		"last_update":       lastUpdate,
	})
}

func (s *Server) getHistory(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{"items": s.history.History()})
}

func (s *Server) update(response http.ResponseWriter, request *http.Request) {
	record, err := s.updates.Update(request.Context(), "api")
	if err != nil {
		s.writeOperationError(response, request, err, record)
		return
	}
	writeJSON(response, http.StatusOK, record)
}

func (s *Server) backups(response http.ResponseWriter, request *http.Request) {
	backups, err := s.updates.ListBackups()
	if err != nil {
		s.writeError(response, request, http.StatusInternalServerError, "backup_list_failed", "无法读取备份列表")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"items": backups})
}

func (s *Server) restore(response http.ResponseWriter, request *http.Request) {
	record, err := s.updates.Restore(request.Context(), request.PathValue("id"), "api-rollback")
	if err != nil {
		s.writeOperationError(response, request, err, record)
		return
	}
	writeJSON(response, http.StatusOK, record)
}

func (s *Server) diagnostics(response http.ResponseWriter, request *http.Request) {
	activeInfo, activeErr := os.Stat(s.options.ActiveConfigPath)
	active := map[string]any{"exists": activeErr == nil}
	if activeErr == nil {
		active["size"] = activeInfo.Size()
		active["modified_at"] = activeInfo.ModTime().UTC()
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	mihomoErr := s.mihomo.Health(ctx)
	writeJSON(response, http.StatusOK, map[string]any{
		"guardian": map[string]any{
			"version": s.options.Version,
			"go":      runtime.Version(),
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		},
		"authentication_enabled": s.auth.Enabled(),
		"scheduler_enabled":      s.options.SchedulerEnabled,
		"active_config":          active,
		"mihomo_reachable":       mihomoErr == nil,
	})
}

func (s *Server) web(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(request.URL.Path, "/api/") {
		s.writeError(response, request, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if s.options.WebDir == "" {
		http.NotFound(response, request)
		return
	}
	relative := strings.TrimPrefix(filepath.Clean("/"+request.URL.Path), "/")
	path := filepath.Join(s.options.WebDir, relative)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		path = filepath.Join(s.options.WebDir, "index.html")
	}
	if _, err := os.Stat(path); err != nil {
		http.NotFound(response, request)
		return
	}
	http.ServeFile(response, request, path)
}

func (s *Server) unauthorized(response http.ResponseWriter, request *http.Request) {
	s.writeError(response, request, http.StatusUnauthorized, "unauthorized", "需要管理员登录")
}

func (s *Server) writeOperationError(response http.ResponseWriter, request *http.Request, err error, record state.Record) {
	switch {
	case errors.Is(err, updater.ErrBusy):
		s.writeError(response, request, http.StatusConflict, "transaction_busy", "已有配置任务正在执行")
	case errors.Is(err, updater.ErrBackupNotFound):
		s.writeError(response, request, http.StatusNotFound, "backup_not_found", "备份不存在")
	default:
		s.logger.Error("configuration operation failed", "request_id", requestID(request.Context()), "stage", record.Stage, "error", err)
		message := "配置操作失败"
		if record.RolledBack {
			message = "配置操作失败，已自动恢复旧配置"
		}
		writeJSON(response, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"code":        "operation_failed",
				"message":     message,
				"request_id":  requestID(request.Context()),
				"stage":       record.Stage,
				"rolled_back": record.RolledBack,
			},
		})
	}
}

func (s *Server) writeError(response http.ResponseWriter, request *http.Request, status int, code, message string) {
	writeJSON(response, status, map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": requestID(request.Context()),
		},
	})
}

func decodeJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request must contain exactly one JSON object")
	}
	return nil
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

type requestIDKey struct{}

func requestIDs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		id := randomID()
		response.Header().Set("X-Request-ID", id)
		next.ServeHTTP(response, request.WithContext(context.WithValue(request.Context(), requestIDKey{}, id)))
	})
}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func randomID() string {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(response, request)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		writer := &statusWriter{ResponseWriter: response}
		next.ServeHTTP(writer, request)
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		s.logger.Info("http request",
			"request_id", requestID(request.Context()),
			"method", request.Method,
			"path", normalizedPath(request.URL.Path),
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func normalizedPath(path string) string {
	if strings.HasPrefix(path, "/api/v1/backups/") && strings.HasSuffix(path, "/restore") {
		return "/api/v1/backups/{id}/restore"
	}
	return path
}

func recoverPanics(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http handler panic", "request_id", requestID(request.Context()))
				writeJSON(response, http.StatusInternalServerError, map[string]any{
					"error": map[string]string{
						"code":       "internal_error",
						"message":    "服务内部错误",
						"request_id": requestID(request.Context()),
					},
				})
			}
		}()
		next.ServeHTTP(response, request)
	})
}
