package jobworker

import (
	"context"
	"testing"
	"time"

	"jobworker/proto"
)

func TestJobServer_RunQueryStopList(t *testing.T) {
	manager := NewJobWorker()
	server := NewJobServer(manager)

	ctx := context.Background()

	// 1️⃣ 测试 Run
	runResp, err := server.Run(ctx, &proto.RunRequest{
		Cmd:  "sleep 1",
		Name: "TestJob",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if runResp.SessionId == "" {
		t.Fatalf("Expected session ID, got empty")
	}

	// 2️⃣ 测试 Query（立即查可能未完成）
	queryResp, err := server.Query(ctx, &proto.QueryRequest{
		SessionId: runResp.SessionId,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if queryResp.Status != "Running" {
		t.Logf("Expected Running, got %s (可能 sleep 太短)", queryResp.Status)
	}

	// 3️⃣ 等待 1.2s，再查一次状态
	time.Sleep(1200 * time.Millisecond)
	queryResp, _ = server.Query(ctx, &proto.QueryRequest{SessionId: runResp.SessionId})
	if queryResp.Status != "Completed" {
		t.Errorf("Expected Completed, got %s", queryResp.Status)
	}

	// 4️⃣ 测试 List
	listResp, err := server.List(ctx, &proto.ListRequest{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listResp.Jobs) == 0 {
		t.Errorf("Expected at least 1 job in list")
	}

	// 5️⃣ 测试 Stop（虽然已完成）
	stopResp, err := server.Stop(ctx, &proto.StopRequest{
		SessionId: runResp.SessionId,
	})
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	t.Logf("Stop response: %s", stopResp.Status)
}
