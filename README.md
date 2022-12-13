# gRPC Load Balancing

gRPC between client and server. Client sends 10 concurrent `Hello` RPCs, wait for all responses, prints them on output and loops forver.

A request `hello` message contains a client identifier and the request counter, while the response is just an echo with a added prefix, a server identifier that processed and responded on the request.

Client accepts a server address, e.g. dns:///server:50051. It will depend on DNS resolution and will apply client side round-robin load balance in case multiple IPs are resolved.

## Build

```sh
docker build -t grpc-load-balancing/client:1 --target client -f Dockerfile .
docker build -t grpc-load-balancing/server:1 --target server -f Dockerfile .
```
^^^ watch the version number if multiple versions are created

## Deploy to Kind cluster

```sh
kind create cluster --name grpc-load-balancing
```

Load client and server docker images

```sh
kind load docker-image grpc-load-balancing/client:1 --name grpc-load-balancing

kind load docker-image grpc-load-balancing/server:1 --name grpc-load-balancing
```

### Service Type Cluster IP

```sh
kubectl create namespace clusterip
kubectl apply -f deploy/clusterip/server.yaml --namespace clusterip
kubectl apply -f deploy/clusterip/client.yaml --namespace clusterip

kubectl logs -f -l app=client -n clusterip
```

Observe that RPCs are not balanced, aka only one server instance is responding, other server instances are idling.

Clean up

```sh
kubectl delete namespace clusterip
```

### Headless service

```sh
kubectl create namespace headless
kubectl apply -f deploy/headless/server.yaml --namespace headless

## wait a few sec!!!
kubectl apply -f deploy/headless/client.yaml --namespace headless

kubectl logs -f -l app=client -n headless
```

Observe that RPC are balanced between server instances. However lets increase number of server replicas from 3 to 5:

```sh
kubectl patch deploy server --namespace headless -p '"spec": {"replicas": 5}'
```

For example 2 new replicas are (`kubectl get pods -n headless`):
* server-56dc579658-x4746
* server-56dc579658-mjzp7

There is no traffic on them (check their logs or client logs).

Clean up

```sh
kubectl delete namespace headless
```


### Max connection age (+headless)

Setting max age to 10s + grace 20s

```sh
kubectl create namespace maxage
kubectl apply -f deploy/maxage/server.yaml --namespace maxage
kubectl apply -f deploy/maxage/client.yaml --namespace maxage

kubectl logs -f -l app=client -n maxage
# observe for at least 1 minute
```

Observe that RPC are balanced between server instances. Lets increase number of server replicas from 3 to 5:

```sh
kubectl patch deploy server --namespace maxage -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client -n maxage
# observe for at least 1 minute
```

Thanks to max age setting, connection is recreated on the client side and as a side-effect new pods are resolved via dns. So eventually there is traffic on all replicas (check their logs or client logs).

Clean up

```sh
kubectl delete namespace maxage
```

### Proxy

Server still runs as a headless services but without any maxage.
We run a proxy in front of server instances.

```sh
kubectl create namespace proxy
kubectl apply -f deploy/proxy/server.yaml --namespace proxy
kubectl apply -f deploy/proxy/proxy.yaml --namespace proxy
kubectl apply -f deploy/proxy/client.yaml --namespace proxy


kubectl logs -f -l app=client -n proxy
kubectl logs -f -l app=proxy -n proxy
```

Observe that RPC are balanced between all server instances.
Lets increase number of server replicas from 3 to 5:

```sh
kubectl patch deploy server --namespace proxy -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client -n proxy
```

Proxy discoveres new server instances, sets up connections and starts balancing RPCs to them. The proxy brings in a bunch of features. But it will slightly add on latency and could result in even two extra node hops (client[@node1] -> proxy[@node2] -> server[@node3]).

However what if we scale the proxy itself (it's running as a headless service but only with 1 instance), say from 1 to 3 and let's observe logs of newly spawned proxy instances:

```sh
kubectl patch deploy proxy --namespace proxy -p '"spec": {"replicas": 3}'

kubectl logs -f proxy-786cfbbf44-xzhhs -nproxy
kubectl logs -f proxy-786cfbbf44-g9q7p -nproxy
```

There are no logs, newly spawned proxy instances are idling. Let's restart the client pod.

```sh
kubectl delete pod -l app=client -nproxy

kubectl logs -f proxy-786cfbbf44-xzhhs -nproxy
kubectl logs -f proxy-786cfbbf44-g9q7p -nproxy
```

There are now logs.

Conclusion, the traffic needs to be correctly load balanced from the downstream/client/caller side.

Clean up

```sh
kubectl delete namespace proxy
```

## Options:

1. Don't support proxy scaling -- just run one huge proxy :)


Assuming that clients implement rudimentary DNS load balancing and that we have an ingress proxy for server instances:
2. Poor man support for proxy scaling - configure keepAlive connectionMaxAge and *Grace durations to eventually enforce clients to reconnect and re-resolve DNS of the proxy

Without DNS load balancing on clients and no ingress proxy for server instances:
3. Put a proxy as a sidecar next to each client instance.

1. not acceptable
2. depends on client side code; it does not support progressive deployments of the proxy; potentially too many hops unless it is run as a daemon set
3. solves all problems stated at 2 - to be tested how would it support progressive releases (canary / blue-green). Global rate limiting theoretically might be possible.


TODO

* [ ] attempt a deployment with a client side proxy as sidecar and see what possibilities are available for progressive deployments
