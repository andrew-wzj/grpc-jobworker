package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"jobworker/client/clientutil"
	"jobworker/proto"
)

func main() {
	// 1. 加载客户端证书和私钥
	clientCert, err := tls.LoadX509KeyPair("certs/client.crt", "certs/client.key")
	if err != nil {
		log.Fatalf("❌ Failed to load client cert/key: %v", err)
	}

	// 2. 加载 CA 根证书
	caCert, err := ioutil.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatalf("❌ Failed to read CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		log.Fatal("❌ Failed to append CA cert to pool")
	}

	// 3. 构造 TLS 配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS12,
	}

	creds := credentials.NewTLS(tlsConfig)

	// 4. 建立 gRPC 加密连接
	conn, err := grpc.Dial("127.0.0.1:50051", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer conn.Close()

	client := proto.NewJobServiceClient(conn)

	// 5. 创建带身份的 context（mock 密码验证保留）
	ctx, cancel := clientutil.CreateContext("admin", "admin")
	defer cancel()

	// Step 1: Run job
	runResp, err := client.Run(ctx, &proto.RunRequest{
		Cmd:  "for i in {1..5}; do echo line $i; sleep 1; done",
		Name: "StreamTest",
	})
	if err != nil {
		log.Fatalf("❌ Run error: %v", err)
	}
	log.Printf("✅ Run: session_id=%s, status=%s", runResp.GetSessionId(), runResp.GetStatus())

	// Step 2: Stream output
	stream, err := client.StreamOutput(ctx, &proto.StreamRequest{SessionId: runResp.GetSessionId()})
	if err != nil {
		log.Fatalf("❌ Stream error: %v", err)
	}

	log.Println("📡 Streaming output:")
	for {
		reply, err := stream.Recv()
		if err != nil {
			log.Printf("📴 Stream ended: %v", err)
			break
		}
		log.Printf("🪵 %s", reply.GetOutput())
	}

	// Step 3: List all jobs
	listResp, err := client.List(ctx, &proto.Empty{})
	if err != nil {
		log.Fatalf("❌ List error: %v", err)
	}
	log.Println("📋 List of jobs:")
	for _, job := range listResp.GetJobs() {
		log.Printf("🧾 [%s] %s - %s", job.GetJob().GetJobId(), job.GetStatus(), job.GetErrorMsg())
	}
}
