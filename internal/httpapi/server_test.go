package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/andrew-wzj/grpc-jobworker/internal/worker"
)

func newServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	w, err := worker.New(worker.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<h1>ui</h1>")}}
	srv := httptest.NewServer(New(w, token, ui))
	t.Cleanup(func() {
		srv.Close()
		w.Shutdown(context.Background())
	})
	return srv
}

func do(t *testing.T, method, url, token, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestJobLifecycle(t *testing.T) {
	srv := newServer(t, "")

	resp, body := do(t, "POST", srv.URL+"/api/jobs", "", `{"name":"hi","cmd":"echo hello"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var job Job
	json.Unmarshal([]byte(body), &job)
	if job.ID == "" || job.Name != "hi" {
		t.Fatalf("bad job: %s", body)
	}

	// Follow streams until the job finishes.
	_, out := do(t, "GET", srv.URL+"/api/jobs/"+job.ID+"/log?follow=1", "", "")
	if out != "hello\n" {
		t.Fatalf("follow log = %q", out)
	}
	_, out = do(t, "GET", srv.URL+"/api/jobs/"+job.ID+"/log", "", "")
	if out != "hello\n" {
		t.Fatalf("log = %q", out)
	}

	_, body = do(t, "GET", srv.URL+"/api/jobs/"+job.ID, "", "")
	json.Unmarshal([]byte(body), &job)
	if job.Status != "completed" || job.FinishedAt == nil {
		t.Fatalf("get: %s", body)
	}

	_, body = do(t, "GET", srv.URL+"/api/jobs", "", "")
	var list []Job
	json.Unmarshal([]byte(body), &list)
	if len(list) != 1 {
		t.Fatalf("list: %s", body)
	}

	if resp, _ := do(t, "DELETE", srv.URL+"/api/jobs/"+job.ID, "", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", srv.URL+"/api/jobs/"+job.ID, "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: %d", resp.StatusCode)
	}
}

func TestStopAndConflicts(t *testing.T) {
	srv := newServer(t, "")
	_, body := do(t, "POST", srv.URL+"/api/jobs", "", `{"cmd":"sleep 30"}`)
	var job Job
	json.Unmarshal([]byte(body), &job)

	if resp, _ := do(t, "DELETE", srv.URL+"/api/jobs/"+job.ID, "", ""); resp.StatusCode != http.StatusConflict {
		t.Errorf("delete running: %d", resp.StatusCode)
	}
	resp, body := do(t, "POST", srv.URL+"/api/jobs/"+job.ID+"/stop", "", "")
	json.Unmarshal([]byte(body), &job)
	if resp.StatusCode != http.StatusOK || job.Status != "stopped" {
		t.Fatalf("stop: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", srv.URL+"/api/jobs/"+job.ID+"/stop", "", ""); resp.StatusCode != http.StatusConflict {
		t.Errorf("stop twice: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", srv.URL+"/api/jobs", "", `{"cmd":""}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty cmd: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", srv.URL+"/api/jobs", "", `not json`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad json: %d", resp.StatusCode)
	}
}

func TestTokenRequired(t *testing.T) {
	srv := newServer(t, "s3cret")
	if resp, _ := do(t, "GET", srv.URL+"/api/jobs", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", srv.URL+"/api/jobs", "wrong", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", srv.URL+"/api/jobs", "s3cret", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("good token: %d", resp.StatusCode)
	}
	// The UI and health check stay public so the page can ask for the token.
	if resp, body := do(t, "GET", srv.URL+"/", "", ""); resp.StatusCode != http.StatusOK || !strings.Contains(body, "ui") {
		t.Errorf("ui: %d %q", resp.StatusCode, body)
	}
	if resp, _ := do(t, "GET", srv.URL+"/healthz", "", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("healthz: %d", resp.StatusCode)
	}
}

func TestFollowStopsWhenClientLeaves(t *testing.T) {
	srv := newServer(t, "")
	_, body := do(t, "POST", srv.URL+"/api/jobs", "", `{"cmd":"echo start; sleep 30"}`)
	var job Job
	json.Unmarshal([]byte(body), &job)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/jobs/"+job.ID+"/log?follow=1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 6)
	if _, err := io.ReadFull(resp.Body, buf); err != nil || string(buf) != "start\n" {
		t.Fatalf("first chunk = %q, %v", buf, err)
	}
	resp.Body.Close()
	do(t, "POST", srv.URL+"/api/jobs/"+job.ID+"/stop", "", "")
}
