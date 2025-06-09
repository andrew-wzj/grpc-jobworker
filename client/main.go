package main

import (
	"log"

	"jobworker/client/clientutil"
	pb "jobworker/proto"

	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("❌ did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewJobServiceClient(conn)

	ctx, cancel := clientutil.CreateContext("admin", "admin")
	defer cancel()

	// 测试 Run 方法
	res, err := client.Run(ctx, &pb.RunRequest{
		Cmd:  "echo Hello",
		Name: "testJob",
	})
	if err != nil {
		log.Fatalf("❌ could not run: %v", err)
	}

	log.Printf("✅ Client received: session_id=%s, status=%s", res.GetSessionId(), res.GetStatus())
}
