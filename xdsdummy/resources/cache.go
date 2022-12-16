package resources

type Listener struct {
	Name       string
	Address    string
	Port       uint32
	RouteNames []string
}

type Route struct {
	Name     string
	Prefix   string
	Clusters []WeightedCluster
}

type WeightedCluster struct {
	Name   string
	Weight uint32
}

type Cluster struct {
	Name      string
	Endpoints []Endpoint
}

type Endpoint struct {
	UpstreamHost string
	UpstreamPort uint32
}
