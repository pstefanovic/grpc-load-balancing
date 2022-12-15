# east-west gRPC Load Balancing in k8s clusters

## Demo Setup

gRPC between client and server. The client sends ten concurrent `Hello` RPCs, waits for all responses, prints them on
the output and loops forever.

A request is a `hello` message containing a client identifier and the request counter, while the response is just an
echo with an added prefix- a server identifier of the server that processed the request.

Client accepts a server address as ENV, e.g., dns:///server:50051. It will depend on DNS resolution and will apply
client side round-robin load balance in case multiple IPs are resolved.

### Build

```sh
docker build -t grpc-load-balancing/client:1 --target client -f Dockerfile .
docker build -t grpc-load-balancing/server:1 --target server -f Dockerfile .

# used for more advanced options
docker build -t grpc-load-balancing/xds:1 --target xds -f Dockerfile .
```

^^^ watch the version number if multiple versions are created

### Deploy to Kind cluster

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

## Load Balancing Options - Demos

### Service Type Cluster IP

```sh
kubectl create namespace clusterip
kubectl apply -f deploy/clusterip/server.yaml -n clusterip
kubectl apply -f deploy/clusterip/client.yaml -n clusterip

kubectl logs -f -l app=client -n clusterip
```

Observe that RPCs are not balanced, aka only one server instance is responding, and other server instances are idling.

Clean up

```sh
kubectl delete namespace clusterip
```

### Headless service

```sh
kubectl create namespace headless
kubectl apply -f deploy/headless/server.yaml -n headless

## wait a few sec!!!
kubectl apply -f deploy/headless/client.yaml -n headless

kubectl logs -f -l app=client -n headless
```

Observe that RPCs are balanced between server instances.
However, let's increase the number of server replicas from 3 to 5:

```sh
kubectl patch deploy server -n headless -p '"spec": {"replicas": 5}'
```

For example, two new replicas are (`kubectl get pods -n headless`):

* server-56dc579658-x4746
* server-56dc579658-mjzp7

There is no traffic on them (check their logs or client logs).

Clean up

```sh
kubectl delete namespace headless
```

### Max connection age (+headless)

Setting max-age to 10s + grace 20s

```sh
kubectl create namespace maxage
kubectl apply -f deploy/maxage/server.yaml -n maxage
kubectl apply -f deploy/maxage/client.yaml -n maxage

kubectl logs -f -l app=client -n maxage
# observe for at least 1 minute
```

Observe that RPCs are balanced between server instances. Let's increase the number of server replicas from 3 to 5:

```sh
kubectl patch deploy server -n maxage -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client -n maxage
# observe for at least 1 minute
```

Thanks to the max-age setting, a connection is recreated on the client side, and as a side effect, new server instances
are resolved via DNS. So eventually, there is traffic on all server instances (check their logs or client logs).

Clean up

```sh
kubectl delete namespace maxage
```

### Server-Side Ingress Proxy

The server runs as a headless service but without max age, instead run a proxy in front of server instances.

```sh
kubectl create namespace proxy
kubectl apply -f deploy/proxy/server.yaml -n proxy
kubectl apply -f deploy/proxy/proxy.yaml -n proxy
kubectl apply -f deploy/proxy/client.yaml -n proxy


kubectl logs -f -l app=client -n proxy
kubectl logs -f -l app=proxy -n proxy
```

Observe that RPCs are balanced between all server instances.
Let's increase the number of server replicas from 3 to 5:

```sh
kubectl patch deploy server -n proxy -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client -n proxy
```

Proxy discovers new server instances, sets up connections, and balances RPCs. Proxy also slightly adds on
latency and could result in up to two extra node hops (client[@node1] -> proxy[@node2] -> server[@node3]).

However, what if we scale the proxy itself (it's running as a headless service but only with one instance), say from 1
to 3 and let's observe logs of newly spawned proxy instances:

```sh
kubectl patch deploy proxy -n proxy -p '"spec": {"replicas": 3}'

kubectl logs -f proxy-786cfbbf44-xzhhs -nproxy
kubectl logs -f proxy-786cfbbf44-g9q7p -nproxy
```

Newly spawned proxy instances are idling. Let's restart the client pod.

```sh
kubectl delete pod -l app=client -nproxy

kubectl logs -f proxy-786cfbbf44-xzhhs -nproxy
kubectl logs -f proxy-786cfbbf44-g9q7p -nproxy
```

Now all proxy instances are processing traffic.

In conclusion, the traffic must be correctly load-balanced from the downstream/client/caller side.

Clean up

```sh
kubectl delete namespace proxy
```

### Client Side Egress Proxy - Sidecar

The server runs as a headless service, and a proxy is deployed as a sidecar on each client instance. No load-balancing
implementation needed in a client application—also, no proxy on the server side.

```sh
kubectl create namespace clsidecar
kubectl apply -f deploy/clsidecar/server.yaml -n clsidecar
kubectl apply -f deploy/clsidecar/client.yaml -n clsidecar

kubectl logs -f -l app=client --container client -n clsidecar
kubectl logs -f -l app=client --container proxy -n clsidecar
```

Observe that RPCs are balanced between all server instances.
Let's increase the number of server replicas from 3 to 5:

```sh
kubectl patch deploy server -n clsidecar -p '"spec": {"replicas": 5}'

kubectl logs -f -l app=client --container client -n clsidecar
```

Sidecar proxy discovers new server instances, sets up connections, and balances RPCs.
Added hop is local to the pod network namespace, so there are no node hops.
Proxy scales with the client; therefore, there are no scalability issues.

Clean up

```sh
kubectl delete namespace clsidecar
```

### Weighted Traffic Routing ~ Canary

Run two server versions as headless services, and a proxy is deployed as a sidecar on each client instance. No
load-balancing implementation is needed in a client application—also, no proxy on the server side.

```sh
kubectl create namespace canary
kubectl apply -f deploy/canary/server-v1.yaml -n canary
kubectl apply -f deploy/canary/server-v2.yaml -n canary
kubectl apply -f deploy/canary/client.yaml -n canary

kubectl logs -f -l app=client --container client -n canary
```

Observe that RPCs are balanced between server-v1 instances while server-v2 is not getting any traffic.
Let's update the client proxy configuration by adding server-v2 as `lbendpoint` to the server cluster definition:

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

Observe that traffic is shifting to server-v2. Now update the config to a 50/50 ratio:

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

Observe a significant amount of traffic flowing to server-v2. Now remove server-v1 from the `lbendpoints`:

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

This approach demonstrates that blue-green or canary deployments can be achieved with just a client-side egress proxy.
The same result could be achieved by adjusting route configurations (filters section) which also brings header-based
routing out of the box. Header-based routing is helpful for so-called initial preview deployments, sometimes a precursor
to canaries.

The issue, however, is the static proxy configuration - it imposes tight coupling between clients and servers.

Clean up

```sh
kubectl delete namespace canary
```

### xDS Control Plane

[xDS API](https://github.com/cncf/xds) (_x_ discover service, originally from
[envoy](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration)) is an attempt
at a standard API that different proxy products implement to load and adapt into their vendor-specific configuration.
Such API decouples proxies and configuration management and allows altering proxy solutions "seamlessly" (at least in
terms of a configuration management solution).

Configuration could be managed in various ways, for example, by plugging into k8s API and augmenting it using CRDs.
Once created/updated etc., the configuration is fed into proxies through xDS API. Essentially, proxies receive their
traffic configuration from the xDS API Server and are continuously updated.

Here for a control plane using a simplified version - an example of
[xDSServer](https://github.com/stevesloka/envoy-xds-server). It centralizes traffic configuration (uses a file
underneath) and serves it through an xDS API. Proxies are pointed to using xDSServer as the source of configuration.

#### Testing

Run two server versions as headless services, and a proxy is deployed as a sidecar on each client instance.
Run an xdsServer and point sidecar proxy to it.
No load-balancing implementation is needed in a client application—also, no proxy on the server side.

```sh
kubectl create namespace xds

kubectl apply -f deploy/xds/xds.yaml -n xds
kubectl apply -f deploy/xds/server-v1.yaml -n xds
kubectl apply -f deploy/xds/server-v2.yaml -n xds
kubectl apply -f deploy/xds/client.yaml -n xds

kubectl logs -f -l app=client --container client -n xds
```

Observe that the sidecar proxy started and is balancing RPCs between server-v1 instances, while server-v2 is not getting
any traffic.

Let's update the xDSServer configuration and move some traffic to server-v2 instances. So, adding an entry in the
listener routes for server-v2, only 5%:

```yaml
...
routes:
  - name: local_route
    prefix: /
    clusters:
      - name: server-v1
        weight: 95
      - name: server-v2
        weight: 5
```

```sh
kubectl edit configmap xdsconfig -n xds
# trigger configmap re-mount on the xdsserver
# (or wait for the cm to be updated automatically - this is a hit & miss due to some bug in the xdsdummy config watcher)
kubectl delete pod -l app=xds -n xds
```

Observe that a small percentage of traffic is moving to server-v2. In the same fashion, increase traffic to 50% and then
set to 100% and remove server-v1.

```yaml
...
routes:
  - name: local_route
    prefix: /
    clusters:
      - name: server-v1
        weight: 50
      - name: server-v2
        weight: 50
```

```yaml
...
routes:
  - name: local_route
    prefix: /
    clusters:
      - name: server-v2
        weight: 100
```

Observe no traffic is flowing to server-v1.

Clean up

```sh
kubectl delete namespace xds
```

---

## gRPC Load Balancing Options:

1. No proxies other than kube-proxy; recreate connections on each RPC or have client-side DNS balancing and maxage
2. Server-side ingress proxy without scaling support -- just run singular and huge proxy instance :)
3. Server side ingress proxy with poor man proxy scaling - configure keepAlive connectionMaxAge and *Grace durations to
   eventually enforce clients to reconnect and re-resolve the DNS of the proxy. (assuming that clients implement
   rudimentary DNS load balancing)
4. Client side egress proxy as a sidecar (or perhaps as a daemonset)
5. Proxyless-grpc - xDS Load Balancing in grpc
   core ([intro](https://events.istio.io/istiocon-2022/sessions/proxyless-grpc/)
   , [feature overview](https://grpc.github.io/grpc/core/md_doc_grpc_xds_features.html))

Discussion:

1. Not acceptable since weighted traffic routing will be needed, which implies a smarter proxy then kube-proxy
2. Not acceptable
3. Load balancing client->proxy implies a delay (keepAlive) and is interdependent with the client-side code; load
   balancing proxy->server is based on DNS, which implies a delay in service discovery; it's not ideal)
   re-deployments of the proxy are tricky; potential latency increase due to too many node hops
   client@node1 -> proxy@node2 -> server@node3
4. Optimal latency and optimal load balancing behavior (although still relies on DNS load balancing proxy->server) for
   the price of more complicated client deployment but arguably, the sidecar could be transparently
   injected by the infra teams at the time of deployment.

   This option, though, is only viable alongside a properly integrated xDS control plane with an improved setup that
   makes proxies transparent to the client applications running as the main container.

5. Still lots of development in progress - looks like a promising pick for the future

## TODO

* [ ] service-mesh tryouts
    * [ ] istio tryout with only client-side proxy as a sidecar
    * [ ] linkerd tryout ...
