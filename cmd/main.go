package main

import (
	"NotifyProject/service"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"

	"NotifyProject/config"
	"NotifyProject/internal/db"
	"NotifyProject/internal/inbox"
	designpb "NotifyProject/proto/protogen/design"
	pb "NotifyProject/proto/protogen/notify"
)

func main() {

	err := godotenv.Load("../conf/config.env")
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	filePath := os.Getenv("CONFIG_FILE")

	yamlData, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		log.Fatalf("Error unmarshaling YAML: %v", err)
	}

	// Connect to MySQL and run AutoMigrate (only creates missing tables)
	mysqlSvc := db.NewMySQLDetailsSvc(&cfg)
	mysqlSvc.AutoMigrate()
	conn := mysqlSvc.ConnectMySQL()
	defer conn.Close()

	// Initialize the gRPC services
	notifyService := service.NewNotifyServiceServer(conn)
	designService := service.NewDesignServiceServer(conn)

	// Start the gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterNotifyServiceServer(grpcServer, notifyService)
	designpb.RegisterDesignServiceServer(grpcServer, designService)

	lis, err := net.Listen(cfg.GrpcDetails.Network, cfg.GrpcDetails.Address)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	go func() {
		log.Println("Starting gRPC server on", cfg.GrpcDetails.Address)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start the HTTP gateway
	ctx := context.Background()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	if err := designpb.RegisterDesignServiceHandlerFromEndpoint(ctx, mux, cfg.GrpcDetails.Endpoint, opts); err != nil {
		log.Fatalf("Failed to start REST gateway: %v", err)
	}
	if err := pb.RegisterNotifyServiceHandlerFromEndpoint(ctx, mux, cfg.GrpcDetails.Endpoint, opts); err != nil {
		log.Fatalf("Failed to start REST gateway: %v", err)
	}

	store := inbox.NewStore(conn)
	inbox.StartCleanup(store)
	inboxHTTP := inbox.NewHTTP(store)
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/design/inbox") {
			inboxHTTP.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})

	log.Println("Starting REST server on", cfg.HttpDetails.Port)
	if err := http.ListenAndServe(cfg.HttpDetails.Port, root); err != nil {
		log.Fatalf("Failed to serve REST: %v", err)
	}
}
