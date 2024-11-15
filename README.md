
## Decoding gRPC Messages using Envoy

Envoy [External Processing Filter](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3/processing_mode.proto) which decodes and alters gRPC messages.


In this flow, the envoy filter will recieve gRPC messages from clients over TLS, then decode and send an altered message to the gRPC Server.


![images/ext_grpc.png](images/ext_grpc.png)


This sample builds ontop of these articles:

* [Envoy External Processing Filter](https://blog.salrashid.dev/articles/2021/envoy_ext_proc/)
* [gRPC Unary requests the hard way: using protorefelect, dynamicpb and wire-encoding to send messages](https://blog.salrashid.dev/articles/2022/grpc_wireformat/)


Basically, the external filter decode the grpc wireformat message into byte messages using (`"github.com/psanford/lencode"`), then `proto.Unmarshal` that into an actual gRPC message we can inspect.


---

In this demo, given the proto

```proto
syntax = "proto3";

package echo;

service EchoServer {
  rpc SayHelloUnary (EchoRequest) returns (EchoReply) {}
  rpc SayHelloServerStream(EchoRequest) returns (stream EchoReply) {}
}

message EchoRequest {
  string name = 1;
}

message EchoReply {
  string message = 1;
}
```

if the client sends `SayHelloUnary` using  `EchoRequest` with `name=alice`, the filter will alter the payload and send `name=bob` to the grpcServer

if the client sends`SayHelloServerStream` with `name=carol`, the gRPC server will stream two responses back with `message="hi carol"`.  However the filter will alter the final grpc message to the client as `message="hi sally"`


```bash
cd ext_proc/

# start external processing server
go run filter.go

## Start envoy
### docker cp `docker create envoyproxy/envoy-dev:latest`:/usr/local/bin/envoy .
envoy -c envoy_ext_proc.yaml -l debug
```


THen the grpc client and server.

```bash
cd grpc_server/

# run server
go run greeter_server/grpc_server.go --grpcport :50051 

# test client directly to server
go run greeter_client/grpc_client.go --host localhost:50051
    2022/10/19 17:37:46 hi alice
    2022/10/19 17:37:46 hi carol
    2022/10/19 17:37:46 hi carol

# test client via envoy
go run greeter_client/grpc_client.go --host localhost:8081
    2022/10/19 17:37:56 hi bob
    2022/10/19 17:37:57 hi sally
    2022/10/19 17:37:57 hi sally
```


#### Using envoy.filters.http.grpc_field_extraction

The example proxy has an additional filter which extracts values from the grpc request itself:
[envoy.filters.http.grpc_field_extraction](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/grpc_field_extraction/v3/config.proto#grpc-field-extraction-proto)


The specific configuration below reads in the descriptor and sets envoy metadata for the `echoRequest` method

```yaml
          - name: envoy.filters.http.grpc_field_extraction
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.grpc_field_extraction.v3.GrpcFieldExtractionConfig
              descriptor_set: 
                filename: "../grpc_server/echo/echo.proto.pb"
              extractions_by_method:
                echo.EchoServer.SayHelloUnary:
                  request_field_extractions:
                    name: {}
```

envoy logs would show this even before the external processor is called

```log
[2024-11-15 12:31:39.604][326918][debug][http] [source/common/http/conn_manager_impl.cc:1160] [Tags: "ConnectionId":"1","StreamId":"13715990017498254296"] request headers complete (end_stream=false):
':method', 'POST'
':scheme', 'https'
':path', '/echo.EchoServer/SayHelloUnary'
':authority', 'grpc.domain.com'
'content-type', 'application/grpc'
'user-agent', 'grpc-go/1.33.2'
'te', 'trailers'
'grpc-timeout', '997267u'

[2024-11-15 12:31:39.604][326918][debug][filter] [source/extensions/filters/http/grpc_field_extraction/filter.cc:122] [Tags: "ConnectionId":"1","StreamId":"13715990017498254296"] decodeData: data size=12 end_stream=true
[2024-11-15 12:31:39.604][326918][info][misc] [source/extensions/filters/http/grpc_field_extraction/message_converter/message_converter.cc:154] 12 + 0
[2024-11-15 12:31:39.604][326918][info][misc] [source/extensions/filters/http/grpc_field_extraction/message_converter/message_converter.cc:32] Checking buffer limits: actual 12 > limit 268435456?
[2024-11-15 12:31:39.604][326918][info][misc] [source/extensions/filters/http/grpc_field_extraction/message_converter/message_converter.cc:154] 12 + 0
[2024-11-15 12:31:39.604][326918][debug][misc] [./source/extensions/filters/http/grpc_field_extraction/message_converter/stream_message.h:22] owned len(owned_bytes_)=7
[2024-11-15 12:31:39.604][326918][info][misc] [source/extensions/filters/http/grpc_field_extraction/message_converter/message_converter.cc:62] len(parsing_buffer_)=0
[2024-11-15 12:31:39.604][326918][debug][misc] [source/extensions/filters/http/grpc_field_extraction/extractor_impl.cc:47] extracted the following resource values from the name field: list_value {
  values {
    string_value: "alice"
  }
}

[2024-11-15 12:31:39.604][326918][debug][filter] [source/extensions/filters/http/grpc_field_extraction/filter.cc:221] [Tags: "ConnectionId":"1","StreamId":"13715990017498254296"] injected dynamic metadata `envoy.filters.http.grpc_field_extraction` with `fields {
  key: "name"
  value {
    list_value {
      values {
        string_value: "alice"
      }
    }
  }
}
`
```

### Appendix

TODO: see if we can create a wasm filter to do the same (which isn't that easy since we need to decode the wireformat)
- [Envoy WASM Filter](https://github.com/salrashid123/envoy_wasm)
