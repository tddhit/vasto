package store

import (
	"github.com/chrislusf/vasto/pb"
	"fmt"
	"log"
	"github.com/chrislusf/vasto/storage/codec"
	"github.com/dgryski/go-jump"
	"bytes"
)

// BootstrapCopy sends all data if BootstrapCopyRequest's TargetClusterSize==0,
// or sends all data belong to TargetShardId in cluster of TargetClusterSize
func (ss *storeServer) BootstrapCopy(request *pb.BootstrapCopyRequest, stream pb.VastoStore_BootstrapCopyServer) error {

	log.Printf("BootstrapCopy %v", request)

	shard, found := ss.keyspaceShards.getShard(request.Keyspace, shard_id(request.ShardId))
	if !found {
		return fmt.Errorf("BootstrapCopy: %s shard %d not found", request.Keyspace, request.ShardId)
	}

	segment, offset := shard.lm.GetSegmentOffset()

	// println("server", shard.serverId, "shard", shard.id, "segment", segment, "offset", offset)

	targetShardId := int32(request.TargetShardId)
	targetClusterSize := int(request.TargetClusterSize)
	batchSize := 1024
	if targetClusterSize > 0 && targetShardId != int32(request.ShardId) {
		batchSize *= targetClusterSize
	}

	err := shard.db.FullScan(batchSize, func(rows []*pb.KeyValue) error {

		var filteredRows []*pb.KeyValue
		for _, row := range rows {
			if bytes.HasPrefix(row.Key, INTERNAL_PREFIX) {
				continue
			}
			if targetClusterSize > 0 {
				partitionHash := codec.GetPartitionHashFromBytes(row.Value)
				if jump.Hash(partitionHash, targetClusterSize) == targetShardId {
					t := row
					filteredRows = append(filteredRows, t)
				}
			} else {
				filteredRows = append(filteredRows, row)
			}
		}

		t := &pb.BootstrapCopyResponse{
			KeyValues: filteredRows,
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
