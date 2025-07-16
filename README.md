# Linux Server JobManager (gRPC + REST + Web UI)

> This is a small project I created after my university graduation, as a tribute to my journey of learning Linux system programming.

A lightweight job scheduling system based on Go, supporting gRPC, RESTful API, and a modern Web UI. You can submit, track, stop, and delete Linux command jobs, with concurrency control, status classification, and log viewing.

---

## 🚀 Quick Start

1. **Clone the repository and install dependencies**
```bash
git clone https://github.com/andrew-wzj/grpc-jobworker.git
cd grpc-jobworker
go mod tidy
```

2. **Generate gRPC code (optional)**
```bash
buf generate
# Or: protoc --go_out=. --go-grpc_out=. proto/job.proto
```

3. **Start the server**
```bash
go run server/main.go
```
Default address: http://localhost:8080

4. **Batch generate test jobs**
```bash
bash test_jobs.sh
```

5. **Open the Web UI**
Visit in your browser:
```
http://localhost:8080/
```
- Real-time job status classification (Completed/Failed/Running)
- Card-style display, modern tech UI
- Log viewing, deletion, search, filtering

6. **CLI/API usage**
- CLI: `./jobrunner run "test-job" "echo hello world"`
- REST: `curl -X POST http://localhost:8080/run ...`
- gRPC: see proto/job.proto

---

## ✨ Features
- ✅ Run any shell command asynchronously
- ✅ Real-time job status classification (Completed/Failed/Running/Pending)
- ✅ Modern tech-style Web UI, card display
- ✅ Real-time log viewing, download support
- ✅ Job deletion, concurrency limit, status transitions
- ✅ RESTful + gRPC + CLI multi-end support
- ✅ SQLite persistence, supports recovery
- ✅ One-click batch test script (test_jobs.sh)
- ✅ Detailed error logs for troubleshooting

---

## 🖥️ Web UI Preview

- Modern dark UI, card grouping, status badges
- Real-time stats refresh
- Search, filter, delete, view logs

## 🏗️ System Architecture

```mermaid
graph TD
  subgraph Frontend
    A[Web UI (HTML/JS)]
    B[CLI Client]
  end
  subgraph Backend
    C[gRPC/REST API Server]
    D[JobWorker (Go)]
    E[SQLite DB]
    F[Log Files]
  end
  A -- HTTP/REST --> C
  B -- gRPC/REST --> C
  C -- schedule/status --> D
  D -- status/log --> E
  D -- output --> F
  C -- query/delete --> E
```

---

## 🗂️ Directory Structure
```
jobworker/
  ├── client/         # CLI client
  ├── db/             # SQLite persistence
  ├── jobworker/      # Core JobWorker logic
  ├── proto/          # gRPC proto definitions
  ├── server/         # HTTP/gRPC server entry
  ├── web/            # Frontend pages and JS
  ├── test_jobs.sh    # Batch test script
  └── README.md
```

---

## 🛠️ Troubleshooting
- **All jobs failed?**
  - Make sure you have the latest code. The bug where all jobs fail due to premature log file closure has been fixed.
  - Check logs/worker_debug.log and individual job logs for errors.
- **Port already in use?**
  - Kill old process: `lsof -i :8080 | grep LISTEN | awk '{print $2}' | xargs kill -9`
- **No data in frontend?**
  - Hard refresh browser cache (Cmd+Shift+R) to ensure main.js is updated.

---

## 📚 Project Highlights
- Go concurrency + gRPC/REST dual protocol, easy to extend
- Frontend-backend separation, modern UI
- Great for engineering practice, resume, open source
- Can be embedded as a job scheduling microservice in larger systems

---

## 🔒 mTLS Certificates & Security
See the certs/ directory and original instructions below. Supports mutual TLS authentication.

---

## 📖 Main API Documentation

### RESTful API

| Method | Path         | Description   | Params/Body Example           |
|--------|--------------|--------------|------------------------------|
| POST   | /run         | Submit job   | {"name":"Test","cmd":"echo Hello"} |
| POST   | /stop        | Stop job     | {"id":"xxxxxx"}                |
| GET    | /list        | List jobs    | -                            |
| GET    | /log/:id     | View log     | -                            |
| DELETE | /delete/:id  | Delete job   | -                            |

#### Example: Submit a job
```bash
curl -X POST http://localhost:8080/run -H "Content-Type: application/json" -d '{"name":"Test","cmd":"echo Hello"}'
```

### gRPC API

- See proto/job.proto for:
  - `rpc RunJob(RunJobRequest) returns (RunJobReply);`
  - `rpc StopJob(StopJobRequest) returns (StopJobReply);`
  - `rpc QueryJobStatus(QueryJobStatusRequest) returns (QueryJobStatusReply);`
  - `rpc ListJobs(ListJobsRequest) returns (ListJobsReply);`
  - `rpc GetJobLog(GetJobLogRequest) returns (GetJobLogReply);`

> *See proto/job.proto or Swagger/OpenAPI docs (gen/openapiv2/proto/job.swagger.json) for details.*

---

> For more help or custom development, feel free to open an issue or PR!


