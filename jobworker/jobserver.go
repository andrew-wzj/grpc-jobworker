package jobworker

import (
	"context"
	"jobworker/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (s *JobServer) Stop(ctx context.Context, req *proto.JobRequest) (*proto.StatusReply, error) {
	err := s.Manager.Stop(req.SessionId)
	if err != nil {
		return &proto.StatusReply{
			Status: "Failed: " + err.Error(),
		}, nil
	}
	return &proto.StatusReply{
		Status: "Stopped",
	}, nil
}

func (s *JobServer) Query(ctx context.Context, req *proto.JobRequest) (*proto.JobStatus, error) {
	s.Manager.mu.Lock()
	defer s.Manager.mu.Unlock()

	job, exists := s.Manager.Jobs[req.SessionId]
	if !exists {
		return &proto.JobStatus{
			Status:   "Not Found",
			ErrorMsg: "No job found with that ID",
		}, nil
	}

	return &proto.JobStatus{
		Job: &proto.Job{
			JobId: job.ID,
			Name:  job.Name,
			Cmd:   job.CmdStr,
		},
		Status:    job.Status,
		ErrorMsg:  job.ErrorMsg,
		IsRunning: job.Status == "Running",
	}, nil
}

func (s *JobServer) List(ctx context.Context, req *proto.Empty) (*proto.JobStatusList, error) {
	s.Manager.mu.Lock()
	defer s.Manager.mu.Unlock()

	var all []*proto.JobStatus
	for _, job := range s.Manager.Jobs {
		all = append(all, &proto.JobStatus{
			Job: &proto.Job{
				JobId: job.ID,
				Name:  job.Name,
				Cmd:   job.CmdStr,
			},
			Status:    job.Status,
			ErrorMsg:  job.ErrorMsg,
			IsRunning: job.Status == "Running",
		})
	}

	return &proto.JobStatusList{Jobs: all}, nil
}

func (s *JobServer) StreamOutput(req *proto.StreamRequest, stream proto.JobService_StreamOutputServer) error {
	s.Manager.mu.Lock()
	job, exists := s.Manager.Jobs[req.SessionId]
	s.Manager.mu.Unlock()
	if !exists {
		return status.Error(codes.NotFound, "Job not found")
	}

	for output := range job.OutputChan {
		stream.Send(&proto.StreamReply{
			Output:  output,
			IsError: false,
		})
	}
	return nil
}
