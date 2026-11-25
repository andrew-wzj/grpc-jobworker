// Package grpcapi exposes the worker over gRPC.
package grpcapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/andrew-wzj/grpc-jobworker/gen/jobworker/v1"
	"github.com/andrew-wzj/grpc-jobworker/internal/store"
	"github.com/andrew-wzj/grpc-jobworker/internal/worker"
)

// Server implements pb.JobServiceServer.
type Server struct {
	pb.UnimplementedJobServiceServer
	w *worker.Worker
}

// Register adds the job service to s.
func Register(s *grpc.Server, w *worker.Worker) {
	pb.RegisterJobServiceServer(s, &Server{w: w})
}

func (s *Server) Run(_ context.Context, req *pb.RunRequest) (*pb.RunResponse, error) {
	j, err := s.w.Run(req.GetName(), req.GetCmd())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.RunResponse{Job: ToProto(j)}, nil
}

func (s *Server) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	j, err := s.w.Stop(ctx, req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.StopResponse{Job: ToProto(j)}, nil
}

func (s *Server) Get(_ context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	j, err := s.w.Get(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.GetResponse{Job: ToProto(j)}, nil
}

func (s *Server) List(context.Context, *pb.ListRequest) (*pb.ListResponse, error) {
	jobs, err := s.w.List()
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &pb.ListResponse{Jobs: make([]*pb.Job, len(jobs))}
	for i, j := range jobs {
		resp.Jobs[i] = ToProto(j)
	}
	return resp, nil
}

func (s *Server) Delete(_ context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	if err := s.w.Delete(req.GetId()); err != nil {
		return nil, toStatus(err)
	}
	return &pb.DeleteResponse{}, nil
}

func (s *Server) StreamOutput(req *pb.StreamOutputRequest, stream grpc.ServerStreamingServer[pb.StreamOutputResponse]) error {
	err := s.w.Follow(stream.Context(), req.GetId(), func(b []byte) error {
		return stream.Send(&pb.StreamOutputResponse{Data: b})
	})
	return toStatus(err)
}

// ToProto converts a stored job to its protobuf form.
func ToProto(j store.Job) *pb.Job {
	out := &pb.Job{
		Id:        j.ID,
		Name:      j.Name,
		Cmd:       j.Cmd,
		Status:    statusToProto[j.Status],
		ExitCode:  int32(j.ExitCode),
		Error:     j.Error,
		CreatedAt: timestamppb.New(j.CreatedAt),
	}
	if !j.StartedAt.IsZero() {
		out.StartedAt = timestamppb.New(j.StartedAt)
	}
	if !j.FinishedAt.IsZero() {
		out.FinishedAt = timestamppb.New(j.FinishedAt)
	}
	return out
}

var statusToProto = map[store.Status]pb.JobStatus{
	store.StatusPending:     pb.JobStatus_JOB_STATUS_PENDING,
	store.StatusRunning:     pb.JobStatus_JOB_STATUS_RUNNING,
	store.StatusCompleted:   pb.JobStatus_JOB_STATUS_COMPLETED,
	store.StatusFailed:      pb.JobStatus_JOB_STATUS_FAILED,
	store.StatusStopped:     pb.JobStatus_JOB_STATUS_STOPPED,
	store.StatusInterrupted: pb.JobStatus_JOB_STATUS_INTERRUPTED,
}

func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, worker.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, worker.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, worker.ErrActive), errors.Is(err, worker.ErrNotActive):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(codes.Internal, err.Error())
}

// TokenAuth returns interceptors that require "authorization: Bearer <token>"
// metadata on every call.
func TokenAuth(token string) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	check := func(ctx context.Context) error {
		md, _ := metadata.FromIncomingContext(ctx)
		for _, v := range md.Get("authorization") {
			got, ok := strings.CutPrefix(v, "Bearer ")
			if ok && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
				return nil
			}
		}
		return status.Error(codes.Unauthenticated, "missing or invalid token")
	}
	unary := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if err := check(ctx); err != nil {
			return nil, err
		}
		return h(ctx, req)
	}
	stream := func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if err := check(ss.Context()); err != nil {
			return err
		}
		return h(srv, ss)
	}
	return unary, stream
}

// TokenCredentials sends a bearer token with every call.
type TokenCredentials struct {
	Token string
	// AllowInsecure permits sending the token without TLS (for localhost).
	AllowInsecure bool
}

func (t TokenCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + t.Token}, nil
}

func (t TokenCredentials) RequireTransportSecurity() bool { return !t.AllowInsecure }
