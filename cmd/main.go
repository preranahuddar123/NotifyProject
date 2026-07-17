package main

import (
	"NotifyProject/service"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"

	"NotifyProject/config"
	"NotifyProject/internal/db"
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

	// Connect to MySQL
	mysqlSvc := db.NewMySQLDetailsSvc(&cfg)
	conn := mysqlSvc.ConnectMySQL()
	defer conn.Close()

	// Initialize the gRPC service
	notifyService := service.NewNotifyServiceServer(conn)

	// Start the gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterNotifyServiceServer(grpcServer, notifyService)

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
	if err := pb.RegisterNotifyServiceHandlerFromEndpoint(ctx, mux, cfg.GrpcDetails.Endpoint, opts); err != nil {
		log.Fatalf("Failed to start REST gateway: %v", err)
	}

	log.Println("Starting REST server on", cfg.HttpDetails.Port)
	if err := http.ListenAndServe(cfg.HttpDetails.Port, mux); err != nil {
		log.Fatalf("Failed to serve REST: %v", err)
	}
}
