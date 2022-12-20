# East-West Traffic Management via Gateway API and Istio

**Q: Wait, what's Gateway API?** Read [here](https://gateway-api.sigs.k8s.io/).

**Q: But that's only supporting north-south traffic?** Yes, but it seems to be moving into the mesh direction, glance
through:

* [GAMMA Announcement from SMI camp](https://smi-spec.io/blog/announcing-smi-gateway-api-gamma/)
* [GAMMA initiative](https://gateway-api.sigs.k8s.io/contributing/gamma/)
* [GEP-1324 - Service Mesh in Gateway API](https://gateway-api.sigs.k8s.io/geps/gep-1324/)
* [Istio about GatewayAPI](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/)
* [Linkerd about GatewayAPI](https://buoyant.io/blog/linkerd-and-the-gateway-api) (mind the xDS API rant at the
  beginning)

**Q: Does it only support HTTP routes, what about gRPC?** In general, gRPC can be routed as plain HTTP/2, e.g. at a
"lower" level; implying that some, potentially significant, features are missed out.
Still, [GRPCRoute](https://gateway-api.sigs.k8s.io/api-types/grpcroute/#grpcroute) is on the Gateway API's radar, with a
specification under the experimental channel - [GEP-1016](https://gateway-api.sigs.k8s.io/geps/gep-1016/).

Implementations for the GRPCRoute on Gateway API from individual service mesh products such
as [Istio](https://github.com/istio/istio/pull/41839) or [Linkerd](https://github.com/linkerd/linkerd2/issues/8663) are
yet to come.

## Prep

Demonstrating gRPC load balancing and some traffic management possibilities, specifically weight-based routing with
istio and its implementation of the experimental east-west Gateway API.

### kind

Install [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), create a cluster, and load the demo docker
images:

```sh
kind create cluster --name istio-gateway-mesh

docker build -t grpc-load-balancing/client:1 --target client -f Dockerfile .
docker build -t grpc-load-balancing/server:1 --target server -f Dockerfile .

kind load docker-image grpc-load-balancing/client:1 --name istio-gatway-mesh
kind load docker-image grpc-load-balancing/server:1 --name istio-gatway-mesh
```

### Gateway-API

Install Gateway API CRDs:

```sh
kubectl get crd gateways.gateway.networking.k8s.io || \
  { kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.5.1" | kubectl apply -f -; }
```

### istio

Install `istioctl` - [instructions](https://istio.io/latest/docs/setup/getting-started/#download).

Install minimal istio - just a control plane:

```sh
istioctl install --set profile=minimal -y
```

## Demo

Run two server versions as either Headless or *ClusterIP services, and run a proxy as a sidecar on each client instance.
Proxy is injected by istio on the pod creation. No load-balancing implementation is needed in a client application—also,
no proxy on the server side.

Traffic setup:

* one common Service "server" that selects pods from both versions server-v1 and server-v2;
* client sends its traffic to a "server" service;
* HTTPRoute "server-routes" matches all traffic from the "server" service and directs it to the "server-v1" service.

Deploying server, routes, and client:

```sh
kubectl create namespace istio

kubectl apply -f ./deploy/istio/server-v1.yaml -n istio
kubectl apply -f ./deploy/istio/server-v2.yaml -n istio
kubectl apply -f ./deploy/istio/server-routes.yaml -n istio

kubectl apply -f ./deploy/istio/client.yaml -n istio

kubectl get pods -n istio
kubectl logs -f -l app=client -n istio
```

Observe that the client pod has two containers - client application as the main container and istio-proxy as a sidecar.
Many other things got injected too - labels and annotations related to istio and prometheus, init container for
rewriting iptables etc.

Update server-routes to shift a small percentage of traffic to server-v2 service:

```yaml
...
rules:
  - backendRefs:
      - name: server-v1
        port: 50051
        weight: 95
      - name: server-v2
        port: 50051
        weight: 5
```

Apply the changes:

```sh
kubectl apply -f ./deploy/istio/server-routes.yaml -n istio
```

Observe that a small amount of traffic is flowing to server-v2.

Similarly, update weights to 50, 100, etc., and eventually remove server-v1 from the server-routes.

```yaml
...
rules:
  - backendRefs:
      - name: server-v1
        port: 50051
        weight: 50
      - name: server-v2
        port: 50051
        weight: 50
```

```yaml
...
rules:
  - backendRefs:
      - name: server-v2
        port: 50051
        weight: 100
```

Observe that no traffic is flowing to server-v1 anymore.
Routing configuration, owned by the server deployment, is seamlessly picked up and propagated to all related proxies by
the control plane.

Clean up

```sh
kubectl delete namespace istio
kind delete clusters istio-gateway-mesh
```
