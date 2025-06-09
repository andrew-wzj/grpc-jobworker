package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"jobworker/jobworker"
	"jobworker/proto"
)

// 拦截器：打印用户名和密码
func authInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		log.Printf("🧾 Raw metadata: %+v", md)
		log.Printf("🔐 [认证信息] username: %v, password: %v", md["username"], md["password"])
	} else {
		log.Println("⚠️ 没有 metadata")
	}
	return handler(ctx, req)
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("❌ Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(authInterceptor),
	)

	worker := jobworker.NewJobWorker()
	server := jobworker.NewJobServer(worker)
	proto.RegisterJobServiceServer(grpcServer, server)

	log.Println("🚀 gRPC JobServer running at :50051 (auth enabled)")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("❌ Failed to serve: %v", err)
	}
}
