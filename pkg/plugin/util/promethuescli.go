package util

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog"
)

var PrometheusClient = InitClient()
var NodeIP = map[string]string{
	"node1":  "116.56.140.23",
	"node2":  "116.56.140.108",
	"node3":  "116.56.140.131",
	"master": "116.56.140.105",
}

func InitClient() api.Client {
	client, err := api.NewClient(api.Config{
		Address: "http://116.56.140.23:30213",
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}
	return client
}

func QueryNetUsageByNode(nodeName string) float64 {
	//首先根据client获取v1的api
	v1api := v1.NewAPI(PrometheusClient)
	//创建一个上下文，用于取消或超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	DPrinter("正在执行node %v 的网络使用率查询,该node的ip为%v\n", nodeName, NodeIP[nodeName])
	querystr := fmt.Sprintf("irate(node_network_transmit_bytes_total{instance=~\"%v.*\"}[60m]) > 0", NodeIP[nodeName])
	DPrinter("执行PromQL:" + querystr + "\n")
	result, warnings, err := v1api.Query(ctx, querystr, time.Now())
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	//累加所有的值
	var sum float64
	resultVec := result.(model.Vector)
	for i := 0; i < resultVec.Len(); i++ {
		sum += float64(resultVec[i].Value)
	}
	//转换成Mb/s
	sum = sum / 1024 / 1024
	DPrinter("查询结果: %v\n", sum)
	klog.V(1).Infof("%v 流量查询结果: %v\n", nodeName, sum)
	return sum
}

func QueryCpuUsageByNode(nodeName string) float64 {
	//首先根据client获取v1的api
	v1api := v1.NewAPI(PrometheusClient)
	//创建一个上下文，用于取消或超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	DPrinter("正在执行node %v 的CPU使用率查询,该node的ip为%v\n", nodeName, NodeIP[nodeName])
	//1-avg(irate(node_cpu_seconds_total{mode="idle",instance=~"116.56.140.105.*"}[30m])) by (instance)
	querystr := fmt.Sprintf("1-avg(irate(node_cpu_seconds_total{mode=\"idle\",instance=~\"%v.*\"}[30m])) by (instance)", NodeIP[nodeName])
	DPrinter("执行PromQL:" + querystr + "\n")
	result, warnings, err := v1api.Query(ctx, querystr, time.Now())
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	//累加所有的值
	var sum float64
	resultVec := result.(model.Vector)
	for i := 0; i < resultVec.Len(); i++ {
		sum += float64(resultVec[i].Value)
	}
	DPrinter("查询结果: %v\n", sum)
	klog.V(1).Infof("%v CPU查询结果: %v\n", nodeName, sum)
	return sum

}
