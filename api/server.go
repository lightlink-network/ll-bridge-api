package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/lightlink-network/ll-bridge-api/database"
)

// API server
type Server struct {
	r    chi.Router
	log  *slog.Logger
	db   database.Database
	opts ServerOpts
}

type ServerOpts struct {
	Logger       *slog.Logger
	URI          string
	DatabaseName string
	Port         string
}

// Create API server
func NewServer(opts ServerOpts) (Server, error) {
	s := Server{
		r:    chi.NewRouter(),
		log:  opts.Logger,
		opts: opts,
	}

	db, err := database.NewDatabase(database.DatabaseOpts{
		URI:          opts.URI,
		DatabaseName: opts.DatabaseName,
		Logger:       opts.Logger.With("component", "database"),
	})
	if err != nil {
		return s, err
	}

	s.db = *db

	return s, nil
}

// Load routes into server and
// starts HTTP server
func (s *Server) StartServer() {
	s.log.Info("📡 Server Started. API Server is now listening on http://localhost:" + s.opts.Port)
	s.routes()
	if err := http.ListenAndServe(":"+s.opts.Port, s.r); err != nil {
		s.log.Error("server error", "error", err)
		os.Exit(1)
	}
}

// Turns server into http server
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.r.ServeHTTP(w, r)
}

// Wrap and format responses
type Response struct {
	StatusCode int         `json:"status_code"`
	Err        bool        `json:"error"`
	Response   interface{} `json:"response"`
}

// Returns JSON response to the API user. HTTP status code
// and data must be provided
func JSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.WriteHeader(statusCode)
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		fmt.Fprintf(w, "%s", err.Error())
	}
}

// Returns ann error to the API user
func ERROR(w http.ResponseWriter, statusCode int, err error) {
	w.WriteHeader(statusCode)
	err = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
	if err != nil {
		fmt.Fprintf(w, "%s", err.Error())
	}
}
