# Canaries

## Demo Setup

gRPC between client and server. Client sends 10 concurrent `Hello` RPCs, wait for all responses, prints them on output
and loops forever.

A request is a `hello` message containing a client identifier and the request counter, while the response is just an
echo with an added prefix, a server identifier that processed and responded on the request.

Client accepts a server address as ENV, e.g. dns:///server:50051. It will depend on DNS resolution and will apply client
side round-robin load balance in case multiple IPs are resolved.

## Build

```sh
docker build -t grpc-load-balancing/client:1 --target client -f Dockerfile .
docker build -t grpc-load-balancing/server:1 --target server -f Dockerfile .

# used for more advanced options
docker build -t grpc-load-balancing/xds:1 --target xds -f Dockerfile .
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

# used for more advanced options
kind load docker-image grpc-load-balancing/xds:1 --name grpc-load-balancing
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

Thanks to max age setting, connection is recreated on the client side and as a side-effect new pods are resolved via
dns. So eventually there is traffic on all replicas (check their logs or client logs).

Clean up

```sh
kubectl delete namespace maxage
```

### Server Side Ingress Proxy

Server runs as a headless services but without maxage. We run a proxy in front of server instances.

```sh
kubectl create namespace proxy
kubectl apply -f deploy/proxy/server.yaml --namespace proxy
kubectl apply -f deploy/proxy/proxy.yaml --namespace proxy
kubectl apply -f deploy/proxy/client.yaml --namespace proxy


kubectl logs -f -l app=client -n proxy
kubectl logs -f -l app=proxy -n proxy
```

Observe that RPC are balanced between all server instances.
Let's increase number of server replicas from 3 to 5:

```sh
kubectl patch deploy server --namespace proxy -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client -n proxy
```

Proxy discovers new server instances, sets up connections and starts balancing RPCs to them. But it will slightly add on
latency and could result in even two extra node hops (client[@node1] ->
proxy[@node2] -> server[@node3]).

However, what if we scale the proxy itself (it's running as a headless service but only with 1 instance), say from 1 to
3
and let's observe logs of newly spawned proxy instances:

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

### Client Side Egress Proxy - Sidecar

Server runs as a headless, proxy is deployed as a sidecar on each client instance. No load balancing implementation
needed in client application. No proxy on the server side.

```sh
kubectl create namespace clsidecar
kubectl apply -f deploy/clsidecar/server.yaml --namespace clsidecar
kubectl apply -f deploy/clsidecar/client.yaml --namespace clsidecar

kubectl logs -f -l app=client --container client -n clsidecar
kubectl logs -f -l app=client --container proxy -n clsidecar
```

Observe that RPC are balanced between all server instances.
Let's increase number of server replicas from 3 to 5:

```sh
kubectl patch deploy server --namespace clsidecar -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client --container client -n clsidecar
```

Sidecar proxy discovers new server instances, sets up connections and starts balancing RPCs to them.
Added hop is local to the pod network namespace, aka no added node hops.
Proxy scales with the client, therefore no issues there.

Clean up

```sh
kubectl delete namespace clsidecar
```

### Weighted Traffic Routing ~ Canary

Run 2 server versions as a headless services, proxy is deployed as a sidecar on each client instance. No load
balancing implementation needed in client application. No proxy on the server side.

```sh
kubectl create namespace canary
kubectl apply -f deploy/canary/server-v1.yaml --namespace canary
kubectl apply -f deploy/canary/server-v2.yaml --namespace canary
kubectl apply -f deploy/canary/client.yaml --namespace canary

kubectl logs -f -l app=client --container client -n canary
```

Observe that RPC are balanced between server-v1 instances, while server-v2 is not getting any traffic.
Let's update client proxy configuration by adding server-v2 as lbendpoint to server cluster definition:

```yaml
...
- lb_endpoints:
    - load_balancing_weight: 99
      endpoint:
        address:
          socket_address:
            address: server-v1
            port_value: 50051

    - load_balancing_weight: 1
      endpoint:
        address:
          socket_address:
            address: server-v2
            port_value: 50051
```

```sh
kubectl edit configmap proxyconfig -n canary
## make adjustments and restart the client
kubectl delete pods -l app=client -n canary
```

Observe that traffic is shifting to server-v2. Now update the config to 50/50 ration:

```yaml
...
- lb_endpoints:
    - load_balancing_weight: 99
      endpoint:
        address:
          socket_address:
            address: server-v1
            port_value: 50051

    - load_balancing_weight: 1
      endpoint:
        address:
          socket_address:
            address: server-v2
            port_value: 50051
```

Observe significant amount traffic flowing to server-v2. Now remove server-v1 from the lbendpoints:

```yaml
...
- lb_endpoints:
    - load_balancing_weight: 100
      endpoint:
        address:
          socket_address:
            address: server-v2
            port_value: 50051
```

Observe no traffic flowing to server-v1 anymore.

This approach demonstrates that blue-green or canary deployments can be achieved with just a client side egress proxy.
Same result could be achieved adjusting route configurations (filters section) which also brings header-based routing
out of the box. Header based routing is useful for so-called initial preview deployments, sometimes a precursor to
canaries.

Issue however is the static proxy configuration - it imposes tight coupling between clients and servers.
This begs for control planes.

Clean up

```sh
kubectl delete namespace canary
```

### xDS Control Plane

[xDS API](https://github.com/cncf/xds) (_x_ discover service, originally from
[envoy](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration)) is an attempt
at a standard API that different proxy solutions implement to load and adapt into their vendor specific configuration.
Such API decouples proxies and configuration management and allows altering proxy solutions "seamlessly" (at least in
terms configuration management solution).

Configuration could be managed in various ways, for example plugging into k8s API and augmenting it with CRDs.
Once created/updated etc. configuration is feed into proxies through xDS API. Essentially, proxies receive their traffic
configuration from the xDS API Server and are continuously kept up to date.

Here for a control plane using a simplified version - example of
[xDSServer](https://github.com/stevesloka/envoy-xds-server). It centralizes traffic configuration (uses a file
underneath) and serves it through a xDS API. Proxies are pointed to use xDSServer as source of configuration.

#### Testing

Run 2 server versions as a headless services, proxy is deployed as a sidecar on each client instance.
Run a xdsServer and point sidecar proxy to it.
No load balancing implementation needed in client application. No proxy on the server side.

```sh
kubectl create namespace xds

kubectl apply -f deploy/xds/xds.yaml --namespace xds
kubectl apply -f deploy/xds/server-v1.yaml --namespace xds
kubectl apply -f deploy/xds/server-v2.yaml --namespace xds
kubectl apply -f deploy/xds/client.yaml --namespace xds

kubectl logs -f -l app=client --container client -n xds
```

Observe that sidecar proxy started and is balancing RPCs between server-v1 instances, while server-v2 is not getting any
traffic.

...

---

## Options:

1. Server side ingress proxy without scaling support -- just run singular and huge proxy instance :)
2. Server side ingress proxy with poor man proxy scaling - configure keepAlive connectionMaxAge and *Grace durations to
   eventually enforce
   clients to reconnect and re-resolve DNS of the proxy. (assuming that clients implement rudimentary DNS load
   balancing)
3. Client side egress proxy as a sidecar (or perhaps as a daemonset)

Discussion:

1. not acceptable
2. load balancing client->proxy implies a delay (keepAlive) and is interdependent with the client side code; load
   balancing proxy->server is based on dns which implies a delay in service discovery, it's not ideal)
   re-deployments of the proxy are tricky; potential latency increase due to too many node hops
   client@node1 -> proxy@node2 -> server@node3
3. optimal latency, optimal load balancing behaviour (although still relies on dns load balancing proxy->server) for the
   price of more complicated client deployment but arguably the sidecar could be transparently
   injected by the infra teams at the time of deployment

## TODO

* [ ] client side proxy as sidecar
    * [x] client sidecar deployment
    * [ ] options for progressive deployments of servers (static & dynamic)
    * [ ] options for global rate limiting
