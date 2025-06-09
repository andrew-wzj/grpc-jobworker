package jobworker

import (
	"context"
	"jobworker/proto"
)

// JobServer implements the gRPC JobServiceServer interface
type JobServer struct {
	proto.UnimplementedJobServiceServer
	Manager *JobWorker
}

func NewJobServer(manager *JobWorker) *JobServer {
	return &JobServer{Manager: manager}
}

func (s *JobServer) Run(ctx context.Context, req *proto.RunRequest) (*proto.RunReply, error) {
	sessionID, err := s.Manager.Run(req.Cmd, req.Name)
	status := "Started"
	if err != nil {
		status = "Failed: " + err.Error()
	}
	return &proto.RunReply{
		SessionId: sessionID,
		Status:    status,
	}, nil
}

func (s *JobServer) Stop(ctx context.Context, req *proto.StopRequest) (*proto.StopReply, error) {
	err := s.Manager.Stop(req.SessionId)
	if err != nil {
		return &proto.StopReply{
			Status: "Failed: " + err.Error(),
		}, nil
	}
	return &proto.StopReply{
		Status: "Stopped",
	}, nil
}

func (s *JobServer) Query(ctx context.Context, req *proto.QueryRequest) (*proto.QueryReply, error) {
	s.Manager.mu.Lock()
	defer s.Manager.mu.Unlock()

	job, exists := s.Manager.Jobs[req.SessionId]
	if !exists {
		return &proto.QueryReply{
			SessionId: req.SessionId,
			Status:    "Not Found",
			ErrorMsg:  "No job found with that ID",
		}, nil
	}

	return &proto.QueryReply{
		SessionId: job.ID,
		Cmd:       job.CmdStr,
		Status:    job.Status,
		ErrorMsg:  job.ErrorMsg,
	}, nil
}

func (s *JobServer) List(ctx context.Context, req *proto.ListRequest) (*proto.ListReply, error) {
	s.Manager.mu.Lock()
	defer s.Manager.mu.Unlock()

	var all []*proto.QueryReply
	for _, job := range s.Manager.Jobs {
		all = append(all, &proto.QueryReply{
			SessionId: job.ID,
			Cmd:       job.CmdStr,
			Status:    job.Status,
			ErrorMsg:  job.ErrorMsg,
		})
	}

	return &proto.ListReply{Jobs: all}, nil
}
