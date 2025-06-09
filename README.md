# 🚀 gRPC JobWorker

A lightweight gRPC-based job management system in Go. 
Submit shell commands, query their status, stop them, or list all—fully asynchronous, metadata-authenticated, and ready for TLS upgrade.

---

## 🔧 Features

- ✅ Run shell commands as jobs (async)
- 📡 Query/Stop/List jobs by session ID
- 🔐 Metadata-based authentication (username/password)
- 🧪 Unit-tested Job logic and gRPC server
- 🖥 CLI client included

---

## 🗂️ Project Structure
grpc-jobworker/
├── client/ # CLI gRPC client
│ ├── main.go
│ └── clientutil/ # createContext helper for auth
│
├── server/ # gRPC server entry
│ └── main.go
│
├── jobworker/ # Core job logic
│ ├── jobworker.go # JobWorker (run/stop/query/list)
│ ├── jobserver.go # gRPC server impl
│ └── jobworker_test.go # Unit test
│
├── proto/ # Protobuf definition
│ └── job.proto
│
├── go.mod / go.sum
└── README.md


---

## 🚀 Getting Started

### 1. Clone the Repo

```bash
git clone https://github.com/andrew-wzj/grpc-jobworker.git
cd grpc-jobworker
go mod tidy

### 2. Generate gRPC Code
protoc --go_out=. --go-grpc_out=. proto/job.proto
