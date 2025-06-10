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
<pre> grpc-jobworker/ ├── client/ # CLI gRPC client │ ├── main.go │ └── clientutil/ # createContext helper for auth │ └── createContext.go │ ├── server/ # gRPC server entry │ └── main.go │ ├── jobworker/ # Core job logic │ ├── jobworker.go # JobWorker (run/stop/query/list) │ ├── jobserver.go # gRPC server implementation │ └── jobworker_test.go # Unit tests │ ├── proto/ # Protobuf definition │ ├── job.proto │ └── job.pb.go / job_grpc.pb.go (auto-generated) │ ├── go.mod ├── go.sum └── README.md </pre>


---
🔧 Features
Run a shell command via gRPC and get a unique session ID

Query job status using the session ID

Stop a running command before it finishes

List all jobs with status and error info

Secure communication with TLS / mTLS and metadata-based auth

Simple logging and visual progress bars in the terminal

📁 Technologies
Go 1.20+

gRPC (with protobuf)

openssl (for generating TLS certs)

grpcurl (for testing)

Standard Go exec, sync, and context packages

💡 Use Cases
🧪 Teaching or learning gRPC/mTLS/auth

🛠️ Lightweight job runner for CI, devops, or scripting tasks

🔒 Demoing secure RPC patterns in a Go environment

🧰 Foundation for building a distributed task execution platform

## 🚀 Getting Started

### 1. Clone the Repo

```bash
git clone https://github.com/andrew-wzj/grpc-jobworker.git
cd grpc-jobworker
go mod tidy

### 2. Generate gRPC Code
protoc --go_out=. --go-grpc_out=. proto/job.proto
