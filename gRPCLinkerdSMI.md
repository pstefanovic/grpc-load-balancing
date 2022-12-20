# East-West Traffic Management via SMI and Linkerd

**Q: What is SMI?** The goal of the [Service Mesh Interface](https://smi-spec.io/) (SMI) is to provide a _common,
portable set_ of service mesh APIs, which a Kubernetes user can use in a provider-agnostic manner. In short, it is an
attempt to create a standard API spec. In practice, the standardization didn't come through, and there has been little
activity on the spec in the past two years:

* istio support is unclear - ([initial repo](https://github.com/servicemeshinterface/smi-adapter-istio),
  ref [issue](https://github.com/servicemeshinterface/smi-adapter-istio/issues/96)),
  ([next iteration](https://github.com/servicemeshinterface/istio-smi-controller) does not look finished)
* linkerd supports SMI's TrafficSplit spec through its [extension](https://linkerd.io/2.12/tasks/linkerd-smi/) model

In any case, the SMI initiative brought about shared understanding between mesh providers. One might think
of it as a precursor to the [GAMMA initiative](https://gateway-api.sigs.k8s.io/contributing/gamma/).

**Q: SMI is not a thing anymore; why demo it?** Maybe, still, it's good to cover SMI, and in the case of linkerd it
is still the advertised way of doing [traffic splits](https://linkerd.io/2.12/features/traffic-split/). This might not
be the case soon in the future.

## Prep

Demonstrating gRPC load balancing and some traffic management possibilities, specifically weight-based routing with
linkerd and its implementation of SMI.

### kind

Install [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), create a cluster, and load the demo docker
images:

```sh
kind create cluster --name linkerd-smi

docker build -t grpc-load-balancing/client:1 --target client -f Dockerfile .
docker build -t grpc-load-balancing/server:1 --target server -f Dockerfile .

kind load docker-image grpc-load-balancing/client:1 --name linkerd-smi
kind load docker-image grpc-load-balancing/server:1 --name linkerd-smi
```

### linkerd

Install `linkerd` - [instructions](https://linkerd.io/2.12/getting-started/#step-1-install-the-cli).

Install linkerd into the kind cluster:

```sh
linkerd install --crds | kubectl apply -f -
linkerd install | kubectl apply -f -
linkerd check
```

### smi extension

Install `smi` extension/adapter for linkerd:

```sh
curl -sL https://linkerd.github.io/linkerd-smi/install | sh
linkerd smi install | kubectl apply -f -
linkerd smi check
```

## Demo

Run two server versions as ClusterIP services, and run a proxy as a sidecar on each client instance.
Proxy is injected by linkerd on the pod creation. No load-balancing implementation is needed in a client
application—also, no proxy on the server side.

Traffic setup:

* one common Service "server" that selects pods from both versions server-v1 and server-v2;
* client sends its traffic to a "server" service;
* TrafficSplit "server-split" matches all traffic destined for the "server" service and with destination
  overriding it forwards to "server-v1".

Deploying server, traffic split, and client:

```sh
kubectl create namespace link

kubectl apply -f ./deploy/linkerd/server-v1.yaml -n link
kubectl apply -f ./deploy/linkerd/server-v2.yaml -n link
kubectl apply -f ./deploy/linkerd/server-split.yaml -n link

kubectl apply -f ./deploy/linkerd/client.yaml -n link

kubectl get pods -n link
kubectl logs -f -l app=client -n link
```

Observe that the client pod has two containers - client application as the main container and linkerd-proxy as a
sidecar. Many other things got injected too - labels and annotations related to linkerd, init container
for rewriting iptables, etc.

Update server-split to shift a small percentage of traffic to server-v2 service:

```yaml
...
spec:
  service: server
  backends:
    - service: server-v1
      weight: 95
    - service: server-v2
      weight: 5
```

Apply the changes:

```sh
kubectl apply -f ./deploy/linkerd/server-split.yaml -n link
```

Observe that a small amount of traffic is flowing to server-v2.

Similarly, update weights to 50, 100, etc., and eventually remove server-v1 from the server-split.

```yaml
...
spec:
  service: server
  backends:
    - service: server-v1
      weight: 50
    - service: server-v2
      weight: 50
```

```yaml
...
spec:
  service: server
  backends:
    - service: server-v2
      weight: 100
```

Observe that no traffic is flowing to server-v1 anymore.
Routing configuration, owned by the server deployment, is seamlessly picked up and propagated to all related proxies by
the control plane.

Clean up

```sh
kubectl delete namespace link
kind delete clusters linkerd-smi
```
