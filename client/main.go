package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"jobworker/client/clientutil"
	"jobworker/proto"
)

func main() {
	// ✅ 1. 加载客户端证书和私钥（client.crt / client.key）
	clientCert, err := tls.LoadX509KeyPair("certs/client.crt", "certs/client.key")
	if err != nil {
		log.Fatalf("❌ Failed to load client cert/key: %v", err)
	}

	// ✅ 2. 加载 CA 根证书，用于验证服务端身份
	caCert, err := ioutil.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatalf("❌ Failed to read CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		log.Fatal("❌ Failed to append CA cert to pool")
	}

	// ✅ 3. 构造完整 TLS 配置（支持双向验证 mTLS）
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert}, // 客户端身份，用于 mTLS
		RootCAs:      caPool,                        // 服务端身份验证（由 CA 签发）
		ServerName:   "localhost",                   // 必须与 server.crt CN 或 SAN 匹配
		MinVersion:   tls.VersionTLS12,
	}

	creds := credentials.NewTLS(tlsConfig)

	// ✅ 4. 建立 gRPC 加密连接
	conn, err := grpc.Dial("127.0.0.1:50051", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer conn.Close()

	client := proto.NewJobServiceClient(conn)

	// ✅ 5. 创建附带认证的 context
	ctx, cancel := clientutil.CreateContext("admin", "admin")
	defer cancel()

	// 🎯 Step 1: Run job
	runResp, err := client.Run(ctx, &proto.RunRequest{
		Cmd:  "sleep 2",
		Name: "SleepJob",
	})
	if err != nil {
		log.Fatalf("❌ Run error: %v", err)
	}
	log.Printf("✅ Run: session_id=%s, status=%s", runResp.GetSessionId(), runResp.GetStatus())

	// ⏳ Step 2: 查询状态
	time.Sleep(1 * time.Second)
	queryResp, err := client.Query(ctx, &proto.QueryRequest{
		SessionId: runResp.GetSessionId(),
	})
	if err != nil {
		log.Fatalf("❌ Query error: %v", err)
	}
	log.Printf("🔍 Query: id=%s, status=%s, error=%s",
		queryResp.GetSessionId(),
		queryResp.GetStatus(),
		queryResp.GetErrorMsg(),
	)

	// 📋 Step 3: 列出全部任务
	listResp, err := client.List(ctx, &proto.ListRequest{})
	if err != nil {
		log.Fatalf("❌ List error: %v", err)
	}
	log.Println("📋 List of jobs:")
	for _, job := range listResp.Jobs {
		log.Printf("🧾 [%s] %s - %s", job.SessionId, job.Status, job.ErrorMsg)
	}
}
