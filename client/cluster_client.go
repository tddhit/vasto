package client

import (
	"github.com/chrislusf/vasto/topology/cluster_listener"
	"github.com/chrislusf/vasto/topology"
	"fmt"
	"github.com/chrislusf/vasto/pb"
	"sync"
)

type ClusterClient struct {
	Master          string
	DataCenter      string
	keyspace        string
	ClusterListener *cluster_listener.ClusterListener
}

func (c *ClusterClient) GetCluster() (*topology.Cluster, error) {
	cluster, found := c.ClusterListener.GetCluster(c.keyspace)
	if !found {
		return nil, fmt.Errorf("no keyspace %s", c.keyspace)
	}
	return cluster, nil
}

// sendRequestsToOneShard send the requests to one shard
// assuming the requests going to the same shard
func (c *ClusterClient) sendRequestsToOneShard(requests []*pb.Request, options []topology.AccessOption) (results []*pb.Response, err error) {

	if len(requests) == 0 {
		return nil, nil
	}

	shardId := requests[0].ShardId

	conn, _, err := c.ClusterListener.GetConnectionByShardId(c.keyspace, int(shardId), options...)

	if err != nil {
		return nil, err
	}

	responses, err := pb.SendRequests(conn, &pb.Requests{
		Keyspace: c.keyspace,
		Requests: requests,
	})
	conn.Close()

	if err != nil {
		return nil, fmt.Errorf("shard %d process error: %v", shardId, err)
	}

	results = responses.Responses

	return

}

// send requests to the cluster's different shards
func (c *ClusterClient) batchProcess(
	requests []*pb.Request,
	options []topology.AccessOption,
	processResultFunc func([] *pb.Response, error) error,
) error {

	cluster, err := c.GetCluster()
	if err != nil {
		return err
	}

	shardIdToRequests := make(map[uint32][]*pb.Request)
	for _, req := range requests {
		req.ShardId = uint32(cluster.FindShardId(req.GetPartitionHash()))
		shardIdToRequests[req.ShardId] = append(shardIdToRequests[req.ShardId], req)
	}

	err = mapEachShard(shardIdToRequests, func(shardId uint32, requests []*pb.Request) error {

		responses, err := c.sendRequestsToOneShard(requests, options)

		if err != nil {
			return fmt.Errorf("shard %d process error: %v", shardId, err)
		}

		if processResultFunc != nil {
			return processResultFunc(responses, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("process error: %v", err)
	}

	return nil

}

func mapEachShard(buckets map[uint32][]*pb.Request, eachFunc func(uint32, []*pb.Request) error) (err error) {
	var wg sync.WaitGroup
	for shardId, requests := range buckets {
		wg.Add(1)
		go func(shardId uint32, requests []*pb.Request) {
			defer wg.Done()
			if eachErr := eachFunc(shardId, requests); eachErr != nil {
				err = eachErr
			}
		}(shardId, requests)
	}
	wg.Wait()
	return
}
