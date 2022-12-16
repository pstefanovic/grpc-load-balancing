# gRPC Load Balancing Basics

Run through a few demonstrations of gRPC load balancing, covering common pitfalls and leading into the usefulness of
proxies and control planes.

## Prep

### kind

Install [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), create a cluster, and load the demo docker
images:

```sh
kind create cluster --name grpc-load-balancing

docker build -t grpc-load-balancing/client:1 --target client -f Dockerfile .
docker build -t grpc-load-balancing/server:1 --target server -f Dockerfile .
docker build -t grpc-load-balancing/xds:1 --target xds -f Dockerfile .

kind load docker-image grpc-load-balancing/client:1 --name grpc-load-balancing
kind load docker-image grpc-load-balancing/server:1 --name grpc-load-balancing
kind load docker-image grpc-load-balancing/xds:1 --name grpc-load-balancing
```

## Demos

### Service Type Cluster IP

The server runs as a **Cluster-IP service**. Before opening a connection to the server, the client runs a DNS resolution
on the server service name, which results in a single IP address.

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

The server runs as a **headless service**. Before opening a connection to the server, the client runs a DNS resolution
on the server service name, which results in multiple IP addresses, one for each server instance - the client runs a
round-robin balancing between resolved IPs.

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

For example, two new instances are (`kubectl get pods -n headless`):

* server-56dc579658-x4746
* server-56dc579658-mjzp7

There is no traffic on them (check their logs or client logs). The client will not be aware of the new server instances
until it re-resolves DNS on the server service name - by default, that happens only on connection creation.

Clean up

```sh
kubectl delete namespace headless
```

### Max connection age (+headless)

The server runs as a headless service. Before opening a connection to the server, the client runs a DNS resolution on
the server service name, which results in multiple IP addresses, one for each server instance - the client runs a
round-robin balancing between resolved IPs.

Setting **max-age to 10s + grace to 20s on the server side** to force reconnection.

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

Thanks to the max-age setting, a connection is eventually recreated on the client side, and as a side effect, new server
instances are resolved via DNS. So ultimately, there is traffic on all server instances (check their logs or client
logs).

Clean up

```sh
kubectl delete namespace maxage
```

### Server-Side Proxy

The server runs as a headless service without max age; instead, run a **proxy in front of server instances**.
Proxy runs as a headless service. Before opening a connection to the proxy, the client runs a DNS resolution on the
proxy service name, potentially resulting in multiple IP addresses, one for each proxy instance - the client, runs a
round-robin balancing between resolved IPs.

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

However, what if we scale the proxy itself (it's running as a headless service but only with one instance)? Let's
increase from 1 to 3 replicas and observe logs of newly spawned proxy instances:

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

After the client restart, connections to the proxy are re-established, which implies re-resolving DNS on the proxy
service name. Now all proxy instances are processing traffic.

In conclusion, the traffic must be correctly load-balanced from the downstream/client/caller side.

Clean up

```sh
kubectl delete namespace proxy
```

### Client Side Proxy - Sidecar

The server runs as a headless service and runs a **proxy as a sidecar on each client instance**. No
load-balancing implementation needed in a client application—also, no proxy on the server side.

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

Run two server versions as headless services and a proxy as a sidecar on each client instance. No
load-balancing implementation is needed in a client application—also, no proxy on the server side.

**Progressively switch traffic** from one server version to another.

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

This approach demonstrates that blue-green or canary deployments of a server can be achieved with just a client-side
proxy. The same result could be achieved by adjusting route configurations (filters section) which also brings
header-based routing out of the box. Header-based routing is helpful for so-called initial preview deployments,
sometimes a precursor to canaries.

The issue, however, is the static proxy configuration - it imposes tight coupling between clients and servers.

Clean up

```sh
kubectl delete namespace canary
```

### Simple xDS Control Plane

[xDS API](https://github.com/cncf/xds) (_x_ discover service, originally from
[envoy](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration)) is an attempt
at a standard API that different proxy products implement to load and adapt into their vendor-specific configuration.
Such API, in theory, decouples proxies and configuration management, improving interoperability; however, altering proxy
either of those would be feat due to various operational reasons.

Configuration could be managed in various ways, for example, by plugging into k8s API and augmenting it using CRDs.
As such, the configuration is fed into proxies through xDS API. Essentially, proxies receive their
traffic configuration from the xDS API Server and are continuously updated.

Here using a simplified version of a control plane - an example of
[xDSServer](https://github.com/stevesloka/envoy-xds-server). It centralizes traffic configuration (uses a file
underneath) and serves it through an xDS API. Proxies are pointed to using xDSServer as the source of configuration.

#### Testing

Run two server versions as headless services, and run a proxy as a sidecar on each client instance.
Run an **xdsServer** and point sidecar proxy to it.
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

## Conclusions

Options:

1. No proxies other than kube-proxy; recreate connections on each RPC or have client-side DNS balancing (either with
   max age or a custom DNS resolver)
2. Server-side proxy with poor man support for proxy scaling - configure keepAlive connectionMaxAge and *Grace durations
   to eventually enforce clients to reconnect and re-resolve the DNS of the proxy. (assuming that clients implement
   rudimentary DNS load balancing)
3. Client-side proxy as a sidecar (or perhaps as a daemonset)
4. Proxyless-grpc - xDS Load Balancing in grpc
   core ([intro](https://events.istio.io/istiocon-2022/sessions/proxyless-grpc/)
   , [feature overview](https://grpc.github.io/grpc/core/md_doc_grpc_xds_features.html))

Discussion:

1. Not acceptable, too much custom logic on the server or client side to ensure rudimentary load balancing or, worse any
   kind of traffic management, such as weight-based routing
2. Load balancing client->proxy implies a delay (keepAlive) and is interdependent with the client-side code; load
   balancing proxy->server is based on DNS, which implies a delay in service discovery; it's not ideal)
   re-deployments of the proxy are tricky; potential latency increase due to too many node hops
   client@node1 -> proxy@node2 -> server@node3
3. Optimal latency and optimal load balancing behavior for the price of more complicated client deployment but arguably,
   the infra teams could transparently inject the sidecar during deployment.

   This option, though, is only viable alongside a properly integrated control plane with a setup that
   makes proxies transparent to the client applications.

4. Still in development but lots of features are available - it looks like a good pick in the scope of gRPC. It still
   implies a control plane, and it also runs a sidecar that talks xDS API.
