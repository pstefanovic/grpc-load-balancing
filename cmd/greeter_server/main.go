/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package main implements a server for Greeter service.
package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc/keepalive"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	pb "pstefanovic/grpc-load-balancing/helloworld"
)

var (
	port                  = 50051
	id                    = os.Getenv("NAME")
	maxConnectionAge      = os.Getenv("MAX_CONNECTION_AGE")
	maxConnectionAgeGrace = os.Getenv("MAX_CONNECTION_AGE_GRACE")
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedGreeterServer
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", in.GetName())
	time.Sleep(time.Second)
	return &pb.HelloReply{Message: id + ": Hello " + in.GetName()}, nil
}

func main() {

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var s *grpc.Server
	if maxConnectionAge != "" && maxConnectionAgeGrace != "" {

		age, err := time.ParseDuration(maxConnectionAge)
		if err != nil {
			log.Fatalf("failed to parse max connection age: %v", err)
		}

		grace, err := time.ParseDuration(maxConnectionAgeGrace)
		if err != nil {
			log.Fatalf("failed to parse max connection age grace: %v", err)
		}

		s = grpc.NewServer(
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionAge:      age,
				MaxConnectionAgeGrace: grace,
			}),
		)
	} else {
		s = grpc.NewServer()
	}

	pb.RegisterGreeterServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
