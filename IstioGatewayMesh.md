# Gateway API + Istio for east-west traffic

Wait what's Gateway API? Read [here](https://gateway-api.sigs.k8s.io/).

But that's only supporting north-south traffic? Yes, but it seems to be moving into the mesh direction, glance through:

* [GAMMA Announcement from SMI camp](https://smi-spec.io/blog/announcing-smi-gateway-api-gamma/)
* [GAMMA initiative](https://gateway-api.sigs.k8s.io/contributing/gamma/)
* [GEP-1324 - Service Mesh in Gateway API](https://gateway-api.sigs.k8s.io/geps/gep-1324/)
* [Istio about GatewayAPI](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/) - followed here
* [Linkerd about GatewayAPI](https://buoyant.io/blog/linkerd-and-the-gateway-api) (mind the xDS API rant at the
  beginning)

## Setup

Let's demonstrate gRPC load balancing and some traffic management possibilities, specifically Weight based routing with
istio and its implementation of the experimental east-west Gateway API.

### kind

Install [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), create a cluster and build demo docker
images:

```sh
kind create cluster --name istio-gatway-mesh

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
* HTTPRoute "server-routes" that matches all traffic from "server" service and directs it to "server-v1" service.
* client sends its traffic to a "server" Service.

Deploying server, routes and client:

```sh
kubectl create namespace istio

kubectl apply -f ./deploy/istio/server-v1.yaml -n istio
kubectl apply -f ./deploy/istio/server-v2.yaml -n istio
kubectl apply -f ./deploy/istio/server-routes.yaml -n istio

kubectl apply -f ./deploy/istio/client.yaml -n istio

kubectl get pods -n istio
kubectl logs -f -l app=client -n istio
```

Observe that client pod has 2 containers - client application as main container and istio-proxy as a sidecar. Note many
other things injected too - labels and annotations related to istio and prometheus, init container for rewriting
iptables etc.

Update server-routes to shift small percentage of traffic to server-v2 service:

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

Observe that small amount of traffic is flowing to server-v2.

In the same fashion update weights to 50, 100 etc., and eventually remove server-v1 from the server-routes.

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




