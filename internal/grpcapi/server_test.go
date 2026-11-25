package grpcapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/andrew-wzj/grpc-jobworker/gen/jobworker/v1"
	"github.com/andrew-wzj/grpc-jobworker/internal/tlsconfig"
	"github.com/andrew-wzj/grpc-jobworker/internal/worker"
)

// startServer runs a gRPC server on an in-memory listener and returns a dialer.
func startServer(t *testing.T, opts ...grpc.ServerOption) func(...grpc.DialOption) pb.JobServiceClient {
	t.Helper()
	w, err := worker.New(worker.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(opts...)
	Register(srv, w)
	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.Stop()
		w.Shutdown(context.Background())
	})
	return func(dopts ...grpc.DialOption) pb.JobServiceClient {
		dopts = append(dopts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}))
		conn, err := grpc.NewClient("passthrough:///bufnet", dopts...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return pb.NewJobServiceClient(conn)
	}
}

func TestRunStreamGetDelete(t *testing.T) {
	client := startServer(t)(grpc.WithTransportCredentials(insecure.NewCredentials()))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := client.Run(ctx, &pb.RunRequest{Name: "count", Cmd: "for i in 1 2 3; do echo $i; sleep 0.1; done"})
	if err != nil {
		t.Fatal(err)
	}
	id := run.GetJob().GetId()

	stream, err := client.StreamOutput(ctx, &pb.StreamOutputRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, msg.GetData()...)
	}
	if string(out) != "1\n2\n3\n" {
		t.Fatalf("streamed %q", out)
	}

	got, err := client.Get(ctx, &pb.GetRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetJob().GetStatus() != pb.JobStatus_JOB_STATUS_COMPLETED || got.GetJob().GetFinishedAt() == nil {
		t.Fatalf("job = %v", got.GetJob())
	}

	list, err := client.List(ctx, &pb.ListRequest{})
	if err != nil || len(list.GetJobs()) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}

	if _, err := client.Delete(ctx, &pb.DeleteRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, &pb.GetRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("Get after delete: %v", err)
	}
}

func TestStopAndErrorCodes(t *testing.T) {
	client := startServer(t)(grpc.WithTransportCredentials(insecure.NewCredentials()))
	ctx := context.Background()

	if _, err := client.Run(ctx, &pb.RunRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty cmd: %v", err)
	}
	run, _ := client.Run(ctx, &pb.RunRequest{Cmd: "sleep 30"})
	if _, err := client.Delete(ctx, &pb.DeleteRequest{Id: run.GetJob().GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("delete running: %v", err)
	}
	stopped, err := client.Stop(ctx, &pb.StopRequest{Id: run.GetJob().GetId()})
	if err != nil || stopped.GetJob().GetStatus() != pb.JobStatus_JOB_STATUS_STOPPED {
		t.Fatalf("Stop = %v, %v", stopped, err)
	}
	if _, err := client.Stop(ctx, &pb.StopRequest{Id: "missing"}); status.Code(err) != codes.NotFound {
		t.Errorf("stop missing: %v", err)
	}
}

func TestTokenAuth(t *testing.T) {
	unary, stream := TokenAuth("s3cret")
	dial := startServer(t, grpc.UnaryInterceptor(unary), grpc.StreamInterceptor(stream))
	ctx := context.Background()

	noToken := dial(grpc.WithTransportCredentials(insecure.NewCredentials()))
	if _, err := noToken.List(ctx, &pb.ListRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("no token: %v", err)
	}
	wrong := dial(grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(TokenCredentials{Token: "nope", AllowInsecure: true}))
	if _, err := wrong.List(ctx, &pb.ListRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("wrong token: %v", err)
	}
	good := dial(grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(TokenCredentials{Token: "s3cret", AllowInsecure: true}))
	if _, err := good.List(ctx, &pb.ListRequest{}); err != nil {
		t.Errorf("good token: %v", err)
	}
	s, _ := noToken.StreamOutput(ctx, &pb.StreamOutputRequest{Id: "x"})
	if _, err := s.Recv(); status.Code(err) != codes.Unauthenticated {
		t.Errorf("stream without token: %v", err)
	}
}

func TestMutualTLS(t *testing.T) {
	dir := t.TempDir()
	writeTestPKI(t, dir)
	serverTLS, err := tlsconfig.Server(filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key"), filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	dial := startServer(t, grpc.Creds(credentials.NewTLS(serverTLS)))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	withCert, err := tlsconfig.Client(filepath.Join(dir, "ca.crt"), filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key"), "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dial(grpc.WithTransportCredentials(credentials.NewTLS(withCert))).List(ctx, &pb.ListRequest{}); err != nil {
		t.Fatalf("client with certificate: %v", err)
	}

	noCert, _ := tlsconfig.Client(filepath.Join(dir, "ca.crt"), "", "", "localhost")
	if _, err := dial(grpc.WithTransportCredentials(credentials.NewTLS(noCert))).List(ctx, &pb.ListRequest{}); err == nil {
		t.Fatal("client without certificate was accepted")
	}
}

// writeTestPKI writes ca, server and client certificates into dir.
func writeTestPKI(t *testing.T, dir string) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	writePEM(t, filepath.Join(dir, "ca.crt"), "CERTIFICATE", caDER)

	for i, name := range []string{"server", "client"} {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(int64(i + 2)),
			Subject:      pkix.Name{CommonName: name},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			DNSNames:     []string{"localhost"},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, _ := x509.MarshalECPrivateKey(key)
		writePEM(t, filepath.Join(dir, name+".crt"), "CERTIFICATE", der)
		writePEM(t, filepath.Join(dir, name+".key"), "EC PRIVATE KEY", keyDER)
	}
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}
