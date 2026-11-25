package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/andrew-wzj/grpc-jobworker/gen/jobworker/v1"
	"github.com/andrew-wzj/grpc-jobworker/internal/grpcapi"
	"github.com/andrew-wzj/grpc-jobworker/internal/tlsconfig"
)

func client(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	addr := fs.String("addr", envOr("JOBWORKER_ADDR", "127.0.0.1:50051"), "gRPC server address (default $JOBWORKER_ADDR)")
	token := fs.String("token", os.Getenv("JOBWORKER_TOKEN"), "API token (default $JOBWORKER_TOKEN)")
	caFile := fs.String("tls-ca", "", "CA certificate to verify the server (enables TLS)")
	certFile := fs.String("tls-cert", "", "client certificate for mutual TLS")
	keyFile := fs.String("tls-key", "", "client private key for mutual TLS")
	serverName := fs.String("tls-server-name", "localhost", "expected server name in its certificate")
	name := fs.String("name", "", "job name (run only)")
	follow := fs.Bool("follow", false, "stream output and exit with the job's exit code (run only)")
	fs.Parse(args)

	creds := insecure.NewCredentials()
	if *caFile != "" || *certFile != "" {
		cfg, err := tlsconfig.Client(*caFile, *certFile, *keyFile, *serverName)
		if err != nil {
			return err
		}
		creds = credentials.NewTLS(cfg)
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if *token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(grpcapi.TokenCredentials{
			Token: *token, AllowInsecure: *caFile == "" && *certFile == "",
		}))
	}
	conn, err := grpc.NewClient(*addr, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	c := pb.NewJobServiceClient(conn)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	needID := func() (string, error) {
		if fs.NArg() != 1 {
			return "", fmt.Errorf("usage: jobworker %s [flags] ID", cmd)
		}
		return fs.Arg(0), nil
	}

	switch cmd {
	case "run":
		if fs.NArg() == 0 {
			return errors.New("usage: jobworker run [-name N] [-follow] -- CMD...")
		}
		resp, err := c.Run(ctx, &pb.RunRequest{Name: *name, Cmd: strings.Join(fs.Args(), " ")})
		if err != nil {
			return err
		}
		if !*follow {
			fmt.Println(resp.GetJob().GetId())
			return nil
		}
		id := resp.GetJob().GetId()
		fmt.Fprintf(os.Stderr, "job %s started\n", id)
		if err := streamOutput(ctx, c, id); err != nil {
			return err
		}
		got, err := c.Get(ctx, &pb.GetRequest{Id: id})
		if err != nil {
			return err
		}
		if j := got.GetJob(); j.GetStatus() != pb.JobStatus_JOB_STATUS_COMPLETED {
			fmt.Fprintf(os.Stderr, "job %s %s\n", id, statusName(j.GetStatus()))
			if j.GetExitCode() > 0 {
				return exitError(j.GetExitCode())
			}
			return exitError(1)
		}
		return nil

	case "list":
		resp, err := c.List(ctx, &pb.ListRequest{})
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSTATUS\tEXIT\tCREATED\tNAME")
		for _, j := range resp.GetJobs() {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", j.GetId(), statusName(j.GetStatus()), exitText(j),
				j.GetCreatedAt().AsTime().Local().Format(time.DateTime), j.GetName())
		}
		return tw.Flush()

	case "get":
		id, err := needID()
		if err != nil {
			return err
		}
		resp, err := c.Get(ctx, &pb.GetRequest{Id: id})
		if err != nil {
			return err
		}
		printJob(resp.GetJob())
		return nil

	case "logs":
		id, err := needID()
		if err != nil {
			return err
		}
		return streamOutput(ctx, c, id)

	case "stop":
		id, err := needID()
		if err != nil {
			return err
		}
		resp, err := c.Stop(ctx, &pb.StopRequest{Id: id})
		if err != nil {
			return err
		}
		printJob(resp.GetJob())
		return nil

	case "delete":
		id, err := needID()
		if err != nil {
			return err
		}
		_, err = c.Delete(ctx, &pb.DeleteRequest{Id: id})
		return err
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func streamOutput(ctx context.Context, c pb.JobServiceClient, id string) error {
	stream, err := c.StreamOutput(ctx, &pb.StreamOutputRequest{Id: id})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		os.Stdout.Write(msg.GetData())
	}
}

func printJob(j *pb.Job) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "ID:\t%s\n", j.GetId())
	fmt.Fprintf(tw, "Name:\t%s\n", j.GetName())
	fmt.Fprintf(tw, "Command:\t%s\n", j.GetCmd())
	fmt.Fprintf(tw, "Status:\t%s\n", statusName(j.GetStatus()))
	fmt.Fprintf(tw, "Exit code:\t%s\n", exitText(j))
	if j.GetError() != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", j.GetError())
	}
	fmt.Fprintf(tw, "Created:\t%s\n", j.GetCreatedAt().AsTime().Local().Format(time.DateTime))
	if j.GetStartedAt() != nil {
		fmt.Fprintf(tw, "Started:\t%s\n", j.GetStartedAt().AsTime().Local().Format(time.DateTime))
	}
	if j.GetFinishedAt() != nil {
		fmt.Fprintf(tw, "Finished:\t%s\n", j.GetFinishedAt().AsTime().Local().Format(time.DateTime))
	}
	tw.Flush()
}

func statusName(s pb.JobStatus) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "JOB_STATUS_"))
}

func exitText(j *pb.Job) string {
	switch j.GetStatus() {
	case pb.JobStatus_JOB_STATUS_COMPLETED, pb.JobStatus_JOB_STATUS_FAILED:
		return fmt.Sprint(j.GetExitCode())
	}
	return "-"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
