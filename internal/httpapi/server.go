// Package httpapi exposes the worker as a JSON REST API and serves the web UI.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andrew-wzj/grpc-jobworker/internal/store"
	"github.com/andrew-wzj/grpc-jobworker/internal/worker"
)

// Job is the JSON form of a job.
type Job struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Cmd        string     `json:"cmd"`
	Status     string     `json:"status"`
	ExitCode   int        `json:"exit_code"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func toJSON(j store.Job) Job {
	out := Job{
		ID: j.ID, Name: j.Name, Cmd: j.Cmd, Status: string(j.Status),
		ExitCode: j.ExitCode, Error: j.Error, CreatedAt: j.CreatedAt,
	}
	if !j.StartedAt.IsZero() {
		out.StartedAt = &j.StartedAt
	}
	if !j.FinishedAt.IsZero() {
		out.FinishedAt = &j.FinishedAt
	}
	return out
}

// New returns the HTTP handler. If token is non-empty, /api/ requests must
// send "Authorization: Bearer <token>". ui is served at /.
func New(w *worker.Worker, token string, ui fs.FS) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("POST /api/jobs", func(rw http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			Cmd  string `json:"cmd"`
		}
		r.Body = http.MaxBytesReader(rw, r.Body, worker.MaxCmdLen+4096)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(rw, http.StatusBadRequest, "invalid JSON body")
			return
		}
		j, err := w.Run(req.Name, req.Cmd)
		if err != nil {
			writeWorkerError(rw, err)
			return
		}
		writeJSON(rw, http.StatusCreated, toJSON(j))
	})
	api.HandleFunc("GET /api/jobs", func(rw http.ResponseWriter, r *http.Request) {
		jobs, err := w.List()
		if err != nil {
			writeWorkerError(rw, err)
			return
		}
		out := make([]Job, len(jobs))
		for i, j := range jobs {
			out[i] = toJSON(j)
		}
		writeJSON(rw, http.StatusOK, out)
	})
	api.HandleFunc("GET /api/jobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		j, err := w.Get(r.PathValue("id"))
		if err != nil {
			writeWorkerError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, toJSON(j))
	})
	api.HandleFunc("POST /api/jobs/{id}/stop", func(rw http.ResponseWriter, r *http.Request) {
		j, err := w.Stop(r.Context(), r.PathValue("id"))
		if err != nil {
			writeWorkerError(rw, err)
			return
		}
		writeJSON(rw, http.StatusOK, toJSON(j))
	})
	api.HandleFunc("DELETE /api/jobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		if err := w.Delete(r.PathValue("id")); err != nil {
			writeWorkerError(rw, err)
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	})
	// GET /api/jobs/{id}/log returns the output so far.
	// With ?follow=1 it streams output until the job finishes.
	api.HandleFunc("GET /api/jobs/{id}/log", func(rw http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if r.URL.Query().Get("follow") == "" {
			j, err := w.Get(id)
			if err != nil {
				writeWorkerError(rw, err)
				return
			}
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			f, err := os.Open(j.LogPath)
			if errors.Is(err, fs.ErrNotExist) {
				return // job has not started yet
			}
			if err != nil {
				writeWorkerError(rw, err)
				return
			}
			defer f.Close()
			io.Copy(rw, f)
			return
		}
		if _, err := w.Get(id); err != nil {
			writeWorkerError(rw, err)
			return
		}
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Cache-Control", "no-store")
		rc := http.NewResponseController(rw)
		_ = w.Follow(r.Context(), id, func(b []byte) error {
			if _, err := rw.Write(b); err != nil {
				return err
			}
			return rc.Flush()
		})
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", requireToken(token, api))
	mux.HandleFunc("GET /healthz", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Write([]byte("ok\n"))
	})
	mux.Handle("/", http.FileServerFS(ui))
	return securityHeaders(mux)
}

func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeError(rw, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		next.ServeHTTP(rw, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		h := rw.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")
		next.ServeHTTP(rw, r)
	})
}

func writeJSON(rw http.ResponseWriter, code int, v any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	json.NewEncoder(rw).Encode(v)
}

func writeError(rw http.ResponseWriter, code int, msg string) {
	writeJSON(rw, code, map[string]string{"error": msg})
}

func writeWorkerError(rw http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, worker.ErrNotFound):
		writeError(rw, http.StatusNotFound, err.Error())
	case errors.Is(err, worker.ErrInvalid):
		writeError(rw, http.StatusBadRequest, err.Error())
	case errors.Is(err, worker.ErrActive), errors.Is(err, worker.ErrNotActive):
		writeError(rw, http.StatusConflict, err.Error())
	case errors.Is(err, context.Canceled):
		// client went away; nothing useful to send
	default:
		writeError(rw, http.StatusInternalServerError, err.Error())
	}
}
