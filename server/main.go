package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"jobworker/jobworker"
	"jobworker/proto"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ...保持你的 authInterceptor 不变...

// 拦截器：验证并打印用户名和密码
func authInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		log.Println("🎯 --- New Request ---")
		log.Printf("🧾 Raw metadata: %+v", md)

		username := md["username"]
		password := md["password"]

		// 校验用户名和密码是否存在并正确
		if len(username) == 0 || len(password) == 0 ||
			username[0] != "admin" || password[0] != "admin" {
			log.Printf("🚫 认证失败: username=%v, password=%v", username, password)
			return nil, status.Error(codes.Unauthenticated, "invalid username or password")
		}

		log.Printf("🔐 认证通过: username=%s", username[0])
	} else {
		log.Println("⚠️ 没有 metadata")
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	// 继续处理请求
	return handler(ctx, req)
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("❌ Failed to listen: %v", err)
	}

	// Load server cert and key
	cert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
	if err != nil {
		log.Fatalf("❌ Failed to load server cert/key: %v", err)
	}

	// Load CA to verify client
	caCert, err := ioutil.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatalf("❌ Failed to read CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert, // enforce mTLS
		ClientCAs:    caPool,
	})

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(authInterceptor),
	)

	worker := jobworker.NewJobWorker()
	server := jobworker.NewJobServer(worker)
	proto.RegisterJobServiceServer(grpcServer, server)

	log.Println("🚀 gRPC JobServer running at :50051 (mTLS + auth enabled)")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("❌ Failed to serve: %v", err)
	}
}
