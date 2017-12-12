package store

import (
	"github.com/chrislusf/vasto/pb"
	"fmt"
	"log"
)

func (ss *storeServer) BootstrapCopy(request *pb.BootstrapCopyRequest, stream pb.VastoStore_BootstrapCopyServer) error {

	log.Printf("BootstrapCopy %v", request)

	node, found := ss.findDbReplica(request.Keyspace, request.ShardId)
	if !found {
		return fmt.Errorf("shard: %s.%d not found", request.Keyspace, request.ShardId)
	}

	segment, offset := node.lm.GetSegmentOffset()

	// println("server", shard.serverId, "shard", shard.id, "segment", segment, "offset", offset)

	err := node.db.FullScan(1024, func(rows []*pb.KeyValue) error {

		t := &pb.BootstrapCopyResponse{
			KeyValues: rows,
		}
		if err := stream.Send(t); err != nil {
			return err
		}
		return nil
	})

	t := &pb.BootstrapCopyResponse{
		BinlogTailProgress: &pb.BootstrapCopyResponse_BinlogTailProgress{
			Segment: segment,
			Offset:  uint64(offset),
		},
	}
	if err := stream.Send(t); err != nil {
		return err
	}

	return err
}

func (ss *storeServer) findDbReplica(keyspace string, nodeId uint32) (replica *shard, found bool) {

	nodes, found := ss.keyspaceShards.getShards(keyspace)
	if !found {
		return nil, false
	}

	for _, node := range nodes {
		if node.id == int(nodeId) {
			replica = node
			return replica, true
		}
	}
	return nil, false
}
