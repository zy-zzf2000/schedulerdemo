package test

import (
	"ouo-scheduler/pkg/plugin/util"
	"testing"
)

func TestQueryNet(t *testing.T) {
	// QueryNetUsageByNode("node1")
	util.QueryNetUsageByNode("node1")
}

func TestQueryCPU(t *testing.T) {
	// QueryNetUsageByNode("node1")
	util.QueryCpuUsageByNode("node1")
}
