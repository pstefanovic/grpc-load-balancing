# East-West gRPC Load Balancing and Traffic Management in k8s clusters

Bringing together a couple of demonstrations concerning gRPC Load Balancing in k8s clusters.

gRPC load balancing is an interesting topic since it implies L7 load balancer capabilities. Such a
thing is not possible with an out-of-the-box kube-proxy solution, contrasting to REST load balancing.

Why L7 load balancing, shouldn't something like L4 still work? A significant reason is the gRPC's usage of multiplexing
provided by http2 on persistent and long-lived connections. With that, gRPC avoids costs related to connection
recreation and mitigates head-of-line blocking problem ([Wiki](https://en.wikipedia.org/wiki/Head-of-line_blocking)
, [SO](https://stackoverflow.com/questions/45583861/how-does-http2-solve-head-of-line-blocking-hol-issue)),
which in turn significantly reduces the number of open connections needed between an individual client and a server
cluster. On the down-side, connection-based load balancing (such as L4) is not optimal; one needs to introspect the
payload to balance RPCs themselves.

## Basic Setup for Demos

Establishing gRPC communication between a client and a scalable cluster of server instances.

The client sends ten concurrent `Hello` RPCs, waits for all responses, prints them on the output, and loops forever.

A request is a `Hello` message containing a client identifier and the request counter, while the response is just an
echo with an added prefix identifying the server that processed the request.

The client accepts the server's address as part of its configuration, e.g., dns:///server:50051. The client uses DNS
resolution and does round-robin load balancing in case multiple IPs are resolved.

## Demos

* [gRPC Load Balancing Pitfalls and Options](gRPCLoadBalancingBasics.md)
* [gRPC With Istio And Gateway API](gRPCIstioGatewayMesh.md)
* ...

## TODO

* service-mesh tryouts
    * [x] istio tryout with only client-side proxy as a sidecar
    * [ ] linkerd tryout ...
    * [ ] compatibility with north-south routing
    * [ ] rate limiting
