package hello_world

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/gorilla/handlers"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type testServer struct {
	UnimplementedGreeterServer
}

func (s *testServer) SayHello(ctx context.Context, in *HelloRequest) (*HelloReply, error) {
	return &HelloReply{Message: in.Name + " world"}, nil
}

func TestHelloWorld(t *testing.T) {
	// setup grpc grpcServer
	grpcServer := grpc.NewServer()
	listner := bufconn.Listen(bufSize)
	RegisterGreeterServer(grpcServer, &testServer{})
	go grpcServer.Serve(listner)

	// setup proxy server
	gwmux := runtime.NewServeMux()
	mux := handlers.CORS(
		handlers.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPatch, http.MethodPut, http.MethodOptions}),
		handlers.AllowedOrigins([]string{"*"}),
	)(gwmux)

	dialOpts := []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return listner.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	}
	err := RegisterGreeterHandlerFromEndpoint(context.Background(), gwmux, "bufnet", dialOpts)
	if err != nil {
		log.Fatal(err)
	}
	proxyServer := httptest.NewServer(mux)

	// setup http client
	httpClient := resty.New().SetBaseURL(proxyServer.URL)
	marshaler := &runtime.JSONPb{}
	httpClient.JSONMarshal = marshaler.Marshal
	httpClient.JSONUnmarshal = marshaler.Unmarshal

	// setup grpc client
	ctx := context.Background()
	client, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(
			func(ctx context.Context, s string) (net.Conn, error) {
				return listner.Dial()
			},
		),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	greeterClient := NewGreeterClient(client)

	// test
	res, err := greeterClient.SayHello(ctx, &HelloRequest{Name: "test"})
	if err != nil {
		log.Fatal(err)
	}

	want := &HelloReply{Message: "test world"}

	assert.Equal(t, want.Message, res.Message)
	assert.Equal(t, want, res)
}
