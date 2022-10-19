package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/salrashid123/envoy_wasm_grpc_payload/echo"

	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/metadata"
)

/*
protoc --go_out=.    -go_opt=paths=source_relative  --descriptor_set_out=echo/echo.proto.pb   --go-grpc_out=. --go-grpc_opt=paths=source_relative     echo/echo.proto
*/
var (
	grpcport = flag.String("grpcport", ":50051", "grpcport")
	tlsCert  = flag.String("tlsCert", "../certs/grpc_server_crt.pem", "tls Certificate")
	tlsKey   = flag.String("tlsKey", "../certs/grpc_server_key.pem", "tls Key")
	hs       *health.Server

	conn *grpc.ClientConn
)

const (
	address string = ":50051"
)

// server is used to implement echo.EchoServer.
type server struct {
	// Embed the unimplemented server
	echo.UnimplementedEchoServerServer
}

func (s *server) SayHelloUnary(ctx context.Context, in *echo.EchoRequest) (*echo.EchoReply, error) {

	log.Printf("Got rpc: --> %s", in.Name)
	log.Printf("Request ctx %v", ctx)

	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, grpc.Errorf(codes.PermissionDenied, fmt.Sprintf("Could not read inbound metadata"))
	}
	log.Printf("Metadata Headers %v", headers)

	return &echo.EchoReply{Message: fmt.Sprintf("hi [%s]", in.Name)}, nil

}

func (s *server) SayHelloServerStream(in *echo.EchoRequest, stream echo.EchoServer_SayHelloServerStreamServer) error {

	log.Println("Got stream:  -->  ")
	stream.Send(&echo.EchoReply{Message: "Hello " + in.Name})
	stream.Send(&echo.EchoReply{Message: "hello again  " + in.Name})

	return nil
}

func main() {
	flag.Parse()
	if *grpcport == "" {
		flag.Usage()
		log.Fatalf("missing -grpcport flag (:50051)")
	}

	lis, err := net.Listen("tcp", *grpcport)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	sopts := []grpc.ServerOption{}

	if *tlsCert == "" || *tlsKey == "" {
		log.Fatalf("Must set --tlsCert and tlsKey if --insecure flags is not set")
	}
	ce, err := credentials.NewServerTLSFromFile(*tlsCert, *tlsKey)
	if err != nil {
		log.Fatalf("Failed to generate credentials %v", err)
	}
	sopts = append(sopts, grpc.Creds(ce))

	s := grpc.NewServer(sopts...)
	echo.RegisterEchoServerServer(s, &server{})

	log.Printf("Starting server...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
