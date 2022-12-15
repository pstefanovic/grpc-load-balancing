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

// Package main implements a client for Greeter service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/examples/helloworld/helloworld"
)

const (
	DefaultAddr = "localhost:50051"
)

var (
	addr = os.Getenv("SERVER_ADDRESS")
	id   = os.Getenv("NAME")
)

func main() {
	flag.Parse()
	// Set up a connection to the server.
	if addr == "" {
		addr = DefaultAddr
	}

	time.Sleep(12 * time.Second) // hack: give time for the proxy or servers to start up in the demos

	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
	)

	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	reqCount := 0
	for true {
		concurrency := 10
		var wg sync.WaitGroup
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			reqCount++
			go sayHello(conn, fmt.Sprintf("%s_%d", id, reqCount), &wg)
		}
		wg.Wait()
	}

	log.Printf("EXIT")

}

func sayHello(conn *grpc.ClientConn, name string, wg *sync.WaitGroup) {

	c := pb.NewGreeterClient(conn)

	// Contact the server and print out its response
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := c.SayHello(ctx, &pb.HelloRequest{Name: name})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Printf(r.GetMessage())

	wg.Done()
}
