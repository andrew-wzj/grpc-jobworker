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

## 🔐 Mutual TLS (mTLS) 安全通信设置

为了保证客户端与服务端之间通信的安全性，项目实现了 **双向 TLS 认证（mTLS）**，即服务端验证客户端身份，客户端也验证服务端身份。

### 📁 证书结构（已生成于 `certs/` 文件夹）

| 文件             | 用途说明                       |
|------------------|--------------------------------|
| `ca.crt`         | 根证书（CA），用于信任验证     |
| `ca.key`         | 根私钥，仅用于签发证书         |
| `server.crt`     | 服务端证书                     |
| `server.key`     | 服务端私钥                     |
| `client.crt`     | 客户端证书                     |
| `client.key`     | 客户端私钥                     |
| `server.cnf`     | 包含 SAN 扩展的服务端配置文件（可选）|

### 🛠️ 生成证书命令

```bash
# 1. 生成 Root CA
openssl genrsa -out certs/ca.key 4096
openssl req -x509 -new -nodes -key certs/ca.key -subj "/CN=JobWorkerCA" -days 3650 -out certs/ca.crt

# 2. 生成服务端证书（推荐使用 SAN）
openssl genrsa -out certs/server.key 4096
openssl req -new -key certs/server.key -subj "/CN=localhost" -out certs/server.csr

# 若需添加 subjectAltName（推荐）
# certs/server.cnf:
# [req]
# distinguished_name = req_distinguished_name
# [req_distinguished_name]
# [req_ext]
# subjectAltName = @alt_names
# [alt_names]
# DNS.1 = localhost

openssl x509 -req -in certs/server.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial \
-out certs/server.crt -days 3650 -extensions req_ext -extfile certs/server.cnf

# 3. 生成客户端证书
openssl genrsa -out certs/client.key 4096
openssl req -new -key certs/client.key -subj "/CN=jobclient" -out certs/client.csr
openssl x509 -req -in certs/client.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial \
-out certs/client.crt -days 3650


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
