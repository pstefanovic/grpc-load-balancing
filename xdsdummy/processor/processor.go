package processor

import (
	"context"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"math"
	"math/rand"
	"os"
	"strconv"

	"pstefanovic/grpc-load-balancing/xdsdummy/resources"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"

	"pstefanovic/grpc-load-balancing/xdsdummy/xdscache"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/sirupsen/logrus"
	"pstefanovic/grpc-load-balancing/xdsdummy/watcher"
)

type Processor struct {
	cache  cache.SnapshotCache
	nodeID string

	// snapshotVersion holds the current version of the snapshot.
	snapshotVersion int64

	logrus.FieldLogger

	xdsCache xdscache.XDSCache
}

func NewProcessor(cache cache.SnapshotCache, nodeID string, log logrus.FieldLogger) *Processor {
	return &Processor{
		cache:           cache,
		nodeID:          nodeID,
		snapshotVersion: rand.Int63n(1000),
		FieldLogger:     log,
		xdsCache: xdscache.XDSCache{
			Listeners: make(map[string]resources.Listener),
			Clusters:  make(map[string]resources.Cluster),
			Routes:    make(map[string]resources.Route),
			Endpoints: make(map[string]resources.Endpoint),
		},
	}
}

// newSnapshotVersion increments the current snapshotVersion
// and returns as a string.
func (p *Processor) newSnapshotVersion() string {

	// Reset the snapshotVersion if it ever hits max size.
	if p.snapshotVersion == math.MaxInt64 {
		p.snapshotVersion = 0
	}

	// Increment the snapshot version & return as string.
	p.snapshotVersion++
	return strconv.FormatInt(p.snapshotVersion, 10)
}

// ProcessFile takes a file and generates an xDS snapshot
func (p *Processor) ProcessFile(file watcher.NotifyMessage) {

	// Parse file into object
	envoyConfig, err := parseYaml(file.FilePath)
	if err != nil {
		p.Errorf("error parsing yaml file: %+v", err)
		return
	}

	// Parse Listeners
	for _, l := range envoyConfig.Listeners {
		var lRoutes []string
		for _, lr := range l.Routes {
			lRoutes = append(lRoutes, lr.Name)
		}

		p.xdsCache.AddListener(l.Name, lRoutes, l.Address, l.Port)

		for _, r := range l.Routes {

			var clusters []resources.WeightedCluster
			for _, wcluster := range r.Clusters {
				clusters = append(clusters, resources.WeightedCluster{
					Name:   wcluster.Name,
					Weight: wcluster.Weight,
				})
			}

			p.xdsCache.AddRoute(r.Name, r.Prefix, clusters)
		}
	}

	// Parse Clusters
	for _, c := range envoyConfig.Clusters {
		p.xdsCache.AddCluster(c.Name)

		// Parse endpoints
		for _, e := range c.Endpoints {
			p.xdsCache.AddEndpoint(c.Name, e.Address, e.Port)
		}
	}

	// Create the snapshot that we'll serve to Envoy
	snapshot, err := cache.NewSnapshot(
		p.newSnapshotVersion(), // version
		map[resource.Type][]types.Resource{ // resources
			//resource.EndpointType: p.xdsCache.EndpointsContents(),
			resource.ClusterType:  p.xdsCache.ClusterContents(),
			resource.RouteType:    p.xdsCache.RouteContents(),
			resource.ListenerType: p.xdsCache.ListenerContents(),
		})

	if err != nil {
		p.Errorf("snapshot creation failed: %+v\n\n\n%+v", snapshot, err)
		return
	}

	if err := snapshot.Consistent(); err != nil {
		p.Errorf("snapshot inconsistency: %+v\n\n\n%+v", snapshot, err)
		return
	}
	p.Debugf("will serve snapshot %+v", snapshot)

	// Add the snapshot to the cache
	if err := p.cache.SetSnapshot(context.TODO(), p.nodeID, snapshot); err != nil {
		p.Errorf("snapshot error %q for %+v", err, snapshot)
		os.Exit(1)
	}
}
