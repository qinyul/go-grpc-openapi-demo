package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/qinyul/go-grpc-openapi-demo/pb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var interuptSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGINT,
}

type server struct {
	pb.UnimplementedServiceServer
}

func (s *server) CreateItem(ctx context.Context, req *pb.CreateItemRequest) (*pb.GetItemResponse, error) {
	return &pb.GetItemResponse{Id: "123", Name: "test", Description: "test"}, nil
}

func runGrcpcServer(ctx context.Context, waitGroup *errgroup.Group) {
	grpcServer := grpc.NewServer()
	waitGroup.Go(func() error {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		pb.RegisterServiceServer(grpcServer, &server{})
		log.Println("gRPC server listening on :50051")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}

		return nil
	})

	waitGroup.Go(func() error {
		<-ctx.Done()
		slog.Info("graceful shutdown gRPC server")

		grpcServer.GracefulStop()
		slog.Info("gRPC server is stopped")

		return nil
	})

}

func runGatewayServer(ctx context.Context, waitGroup *errgroup.Group) {
	mux := runtime.NewServeMux()

	httpServer := &http.Server{
		Handler: mux,
		Addr:    "localhost:8000",
	}
	waitGroup.Go(func() error {
		err := pb.RegisterServiceHandlerFromEndpoint(ctx, mux, "localhost:50051", []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())})
		if err != nil {
			log.Fatalf("Failed to register handler: %v", err)
		}

		log.Println("HTTP server listening on :8000")
		httpServer.ListenAndServe()

		return nil
	})

	waitGroup.Go(func() error {
		<-ctx.Done()
		slog.Info("graceful shutdown HTTP gateway server")

		err := httpServer.Shutdown(context.Background())
		if err != nil {
			slog.Error("failed to shutdown gateway server")
			return err
		}

		slog.Info("Gateway server is stopped")
		return nil
	})

}

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), interuptSignals...)

	defer stop()

	waitGroup, ctx := errgroup.WithContext(ctx)

	runGatewayServer(ctx, waitGroup)
	runGrcpcServer(ctx, waitGroup)

	err := waitGroup.Wait()
	if err != nil {
		log.Fatal("error from wait group")
	}
}
