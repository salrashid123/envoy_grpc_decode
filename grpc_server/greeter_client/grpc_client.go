package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"time"

	"github.com/salrashid123/envoy_grpc_decode/echo"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	address    = flag.String("host", "localhost:8081", "host:port of gRPC server")
	tlsCACert  = flag.String("cacert", "../certs/tls-ca.crt", "tls CA Certificate")
	serverName = flag.String("servername", "grpc.domain.com", "CACert for server")
)

const (
	defaultName = "world"
)

func main() {
	flag.Parse()

	// Set up a connection to the server.
	var err error
	var conn *grpc.ClientConn

	var tlsCfg tls.Config
	rootCAs := x509.NewCertPool()
	pem, err := ioutil.ReadFile(*tlsCACert)
	if err != nil {
		log.Fatalf("failed to load root CA certificates  error=%v", err)
	}
	if !rootCAs.AppendCertsFromPEM(pem) {
		log.Fatalf("no root CA certs parsed from file ")
	}
	tlsCfg.RootCAs = rootCAs
	tlsCfg.ServerName = *serverName

	ce := credentials.NewTLS(&tlsCfg)
	conn, err = grpc.Dial(*address, grpc.WithTransportCredentials(ce))

	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := echo.NewEchoServerClient(conn)
	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// ******** Unary Request
	r, err := c.SayHelloUnary(ctx, &echo.EchoRequest{Name: "alice"})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Printf("%s", r.Message)

	// // /// ***** SERVER Streaming
	stream, err := c.SayHelloServerStream(ctx, &echo.EchoRequest{Name: "carol"})
	if err != nil {
		log.Fatalf("SayHelloStream(_) = _, %v", err)
	}
	for {
		m, err := stream.Recv()
		if err == io.EOF {
			//t := stream.Trailer()
			//log.Println("Stream Trailer: ", t)
			break
		}
		if err != nil {
			log.Fatalf("SayHelloStream(_) = _, %v", err)
		}

		log.Printf("%s", m.Message)
	}

}
