package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	//"github.com/golang/protobuf/ptypes/wrappers"

	v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/psanford/lencode"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	grpcport = flag.String("grpcport", ":18080", "grpcport")
	tlsCert  = flag.String("tlsCert", "../certs/ext_server.crt", "tls Certificate")
	tlsKey   = flag.String("tlsKey", "../certs/ext_server.key", "tls Key")
)

const ()

type server struct{}

type healthServer struct{}

func (s *healthServer) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	log.Printf("Handling grpc Check request + %s", in.String())
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *healthServer) Watch(in *healthpb.HealthCheckRequest, srv healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watch is not implemented")
}

func (s *server) Process(srv pb.ExternalProcessor_ProcessServer) error {

	log.Println("Got stream:  -->  ")
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot receive stream request: %v", err)
		}

		resp := &pb.ProcessingResponse{}
		switch v := req.Request.(type) {
		case *pb.ProcessingRequest_RequestHeaders:
			log.Printf("pb.ProcessingRequest_RequestHeaders %v \n", v)
			r := req.Request
			h := r.(*pb.ProcessingRequest_RequestHeaders)
			log.Printf("Got RequestHeaders.Attributes %v", h.RequestHeaders.Attributes)
			log.Printf("Got RequestHeaders.Headers %v", h.RequestHeaders.Headers)

			for _, n := range h.RequestHeaders.Headers.Headers {
				if n.Key == ":method" && n.Value == "POST" {
					for _, n := range h.RequestHeaders.Headers.Headers {
						log.Printf("Header %s %s", n.Key, n.Value)

						rhq := &pb.HeadersResponse{
							Response: &pb.CommonResponse{},
						}

						resp = &pb.ProcessingResponse{
							Response: &pb.ProcessingResponse_RequestHeaders{
								RequestHeaders: rhq,
							},
							ModeOverride: &v3.ProcessingMode{
								RequestBodyMode:    v3.ProcessingMode_BUFFERED,
								ResponseHeaderMode: v3.ProcessingMode_SKIP,
								ResponseBodyMode:   v3.ProcessingMode_NONE,
							},
						}
						break

					}

				}
			}

		case *pb.ProcessingRequest_RequestBody:

			r := req.Request
			b := r.(*pb.ProcessingRequest_RequestBody)
			log.Printf("   RequestBody: %s", string(b.RequestBody.Body))
			log.Printf("   EndOfStream: %T", b.RequestBody.EndOfStream)
			if b.RequestBody.EndOfStream {

				dec := lencode.NewDecoder(bytes.NewBuffer(b.RequestBody.Body), lencode.SeparatorOpt([]byte{0}))
				reqMessageBytes, err := dec.Decode()
				if err != nil {
					log.Printf("Error Encode: %v\n", err)
					return err
				}

				echoRequestMessageType, err := protoregistry.GlobalTypes.FindMessageByName("echo.EchoRequest")
				if err != nil {
					log.Printf("Error FindMessageByName: %v\n", err)
					return err
				}

				pmr := echoRequestMessageType.New()

				echoRequestMessageDescriptor := echoRequestMessageType.Descriptor()

				msg := echoRequestMessageDescriptor.Fields().ByName("name")
				err = proto.Unmarshal(reqMessageBytes, pmr.Interface())
				if err != nil {
					log.Printf("Error Unmarshal: %v\n", err)
					return err
				}
				fmt.Printf("Decode echo.EchoRequest payload %s\n", pmr.Get(msg).String())

				//  alter the inbound message if name=alice

				if pmr.Get(msg).String() == "alice" {
					a, err := anypb.New(echoRequestMessageType.New().Interface())
					if err != nil {
						log.Printf("Error Unmarshal: %v\n", err)
						return err
					}

					pmr2 := echoRequestMessageType.New()
					inner_name := echoRequestMessageDescriptor.Fields().ByName("name")
					pmr2.Set(inner_name, protoreflect.ValueOfString("bob"))

					in, err := proto.Marshal(pmr2.Interface())
					if err != nil {
						log.Printf("Error NewEncoder: %v\n", err)
						return err
					}
					fmt.Printf("Encoded EchoRequest using protojson and anypb %v\n", hex.EncodeToString(a.Value))

					var out bytes.Buffer
					enc := lencode.NewEncoder(&out, lencode.SeparatorOpt([]byte{0}))
					if err != nil {
						log.Printf("Error NewEncoder: %v\n", err)
						return err
					}
					err = enc.Encode(in)
					if err != nil {
						log.Printf("Error NewEncoder.Encode: %v\n", err)
						return err
					}

					resp = &pb.ProcessingResponse{
						Response: &pb.ProcessingResponse_RequestBody{
							RequestBody: &pb.BodyResponse{
								Response: &pb.CommonResponse{
									BodyMutation: &pb.BodyMutation{
										Mutation: &pb.BodyMutation_Body{
											Body: out.Bytes(),
										},
									},
								},
							},
						},

						ModeOverride: &v3.ProcessingMode{
							ResponseHeaderMode: v3.ProcessingMode_SEND,
							ResponseBodyMode:   v3.ProcessingMode_NONE,
						},
					}
				} else {
					resp = &pb.ProcessingResponse{
						Response: &pb.ProcessingResponse_RequestBody{},
						ModeOverride: &v3.ProcessingMode{
							ResponseHeaderMode: v3.ProcessingMode_SEND,
							ResponseBodyMode:   v3.ProcessingMode_NONE,
						},
					}
				}

			}

		case *pb.ProcessingRequest_ResponseHeaders:
			log.Printf("pb.ProcessingRequest_ResponseHeaders %v \n", v)
			rhq := &pb.HeadersResponse{}
			resp = &pb.ProcessingResponse{
				Response: &pb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: rhq,
				},
				ModeOverride: &v3.ProcessingMode{
					ResponseBodyMode: v3.ProcessingMode_BUFFERED,
				},
			}

		case *pb.ProcessingRequest_ResponseBody:
			log.Printf("pb.ProcessingRequest_ResponseBody %v \n", v)

			r := req.Request
			b := r.(*pb.ProcessingRequest_ResponseBody)
			log.Printf("   ResponseBody: %s", string(b.ResponseBody.Body))
			log.Printf("   EndOfStream: %T", b.ResponseBody.EndOfStream)
			if b.ResponseBody.EndOfStream {
				enc := lencode.NewDecoder(bytes.NewBuffer(b.ResponseBody.Body), lencode.SeparatorOpt([]byte{0}))

				var bytesToSend []byte
				for {
					respMessageBytes, err := enc.Decode()

					if err != nil {
						if err == io.EOF {
							break
						}
						log.Fatalf("could not Decode  %v", err)
						return err
					}

					echoResponseMessageType, err := protoregistry.GlobalTypes.FindMessageByName("echo.EchoReply")
					if err != nil {
						log.Printf("Error FindMessageByName: %v\n", err)
						return err
					}

					pmr := echoResponseMessageType.New()

					echoResponseMessageDescriptor := echoResponseMessageType.Descriptor()

					msg := echoResponseMessageDescriptor.Fields().ByName("message")
					err = proto.Unmarshal(respMessageBytes, pmr.Interface())
					if err != nil {
						log.Printf("Error Unmarshal: %v\n", err)
						return err
					}
					fmt.Printf("Decoded echo.EchoReply message [%s]\n", pmr.Get(msg).String())

					// if the response is "hi carol, change it to 'hi sally"

					if pmr.Get(msg).String() == "hi carol" {
						pmr2 := echoResponseMessageType.New()
						inner_name := echoResponseMessageDescriptor.Fields().ByName("message")
						pmr2.Set(inner_name, protoreflect.ValueOfString("hi sally"))

						in, err := proto.Marshal(pmr2.Interface())
						if err != nil {
							log.Printf("Error NewEncoder: %v\n", err)
							return err
						}

						var out bytes.Buffer
						lenc := lencode.NewEncoder(&out, lencode.SeparatorOpt([]byte{0}))
						if err != nil {
							log.Printf("Error NewEncoder: %v\n", err)
							return err
						}
						err = lenc.Encode(in)
						if err != nil {
							log.Printf("Error NewEncoder.Encode: %v\n", err)
							return err
						}

						bytesToSend = append(bytesToSend, out.Bytes()...)
					}

				}

				if len(bytesToSend) == 0 {
					resp = &pb.ProcessingResponse{}
				} else {
					resp = &pb.ProcessingResponse{
						Response: &pb.ProcessingResponse_ResponseBody{
							ResponseBody: &pb.BodyResponse{
								Response: &pb.CommonResponse{
									BodyMutation: &pb.BodyMutation{
										Mutation: &pb.BodyMutation_Body{
											Body: bytesToSend,
										},
									},
								},
							},
						},
					}
				}
			}

		default:
			log.Printf("Unknown Request type %v\n", v)
		}
		if err := srv.Send(resp); err != nil {
			log.Printf("send error %v", err)
		}
	}
}

func main() {

	flag.Parse()

	pbFiles := []string{
		"../grpc_server/echo/echo.proto.pb",
	}

	for _, fileName := range pbFiles {

		protoFile, err := ioutil.ReadFile(fileName)
		if err != nil {
			panic(err)
		}

		fileDescriptors := &descriptorpb.FileDescriptorSet{}
		err = proto.Unmarshal(protoFile, fileDescriptors)
		if err != nil {
			panic(err)
		}
		for _, pb := range fileDescriptors.GetFile() {
			var fdr protoreflect.FileDescriptor
			fdr, err = protodesc.NewFile(pb, protoregistry.GlobalFiles)
			if err != nil {
				panic(err)
			}
			fmt.Printf("Loading package %s\n", fdr.Package().Name())
			err = protoregistry.GlobalFiles.RegisterFile(fdr)
			if err != nil {
				panic(err)
			}
			for _, m := range pb.MessageType {

				fmt.Printf("  Registering MessageType: %s\n", *m.Name)
				md := fdr.Messages().ByName(protoreflect.Name(*m.Name))
				mdType := dynamicpb.NewMessageType(md)

				err = protoregistry.GlobalTypes.RegisterMessage(mdType)
				if err != nil {
					panic(err)
				}
			}
		}
	}

	lis, err := net.Listen("tcp", *grpcport)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	sopts := []grpc.ServerOption{}

	if *tlsCert == "" || *tlsKey == "" {
		log.Fatalf("Must set --tlsCert and tlsKey")
	}
	ce, err := credentials.NewServerTLSFromFile(*tlsCert, *tlsKey)
	if err != nil {
		log.Fatalf("Failed to generate credentials %v", err)
	}
	sopts = append(sopts, grpc.Creds(ce))

	s := grpc.NewServer(sopts...)
	pb.RegisterExternalProcessorServer(s, &server{})
	healthpb.RegisterHealthServer(s, &healthServer{})

	log.Printf("Starting server...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}
