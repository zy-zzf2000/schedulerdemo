package ouo

import (
	"context"
	"ouo-scheduler/pkg/plugin/util"
	"strings"

	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

// Name ... the custom shceduler name
const Name = "ouo-scheduler"

// CustomScheduler ... The type CustomScheduler implement the interface of the kube-scheduler framework
type CustomScheduler struct {
	handle              framework.FrameworkHandle
	resourceToWeightMap map[string]float64 //存放CPU、内存与网络权重的map
	cpuCoreNum          map[string]float64 //存放每个CPU核数的map
}

// Let the type CustomScheduler implement the QueueSortPlugin, PreFilterPlugin interface
var _ framework.PreFilterPlugin = &CustomScheduler{}
var _ framework.ScorePlugin = &CustomScheduler{}
var _ framework.ScoreExtensions = &CustomScheduler{}

type NetResourceMap struct {
	mmap map[string]float64
}

// 根据文档，Clone方法需要实现浅拷贝
func (m *NetResourceMap) Clone() framework.StateData {
	c := &NetResourceMap{
		mmap: m.mmap,
	}
	return c
}

func (*CustomScheduler) Name() string {
	return Name
}

// PreFilter ... Implement PreFilterPlugin interface PreFilter()
func (n *CustomScheduler) PreFilter(ctx context.Context, state *framework.CycleState, p *v1.Pod) *framework.Status {
	/*
		TODO:初始化集群中所有节点的网络资源的Request(已经申请的网络资源)和Capacity(节点的网络资源总量)
		在一个新的集群中，所有节点的Request都为0，Capacity为节点的网络资源总量
		分别使用两个Map对象NodeNetRequestMap和NodeNetCapacityMap存储所有节点的网络Request和Capacity
		然后将这两个Map对象存储到CycleState中
	*/
	klog.V(1).Infof("enter prefilter pod: %v\n", p.Name)
	//获取所有节点的名称，初始化每个Node的Request和Capacity
	nodeList, err := n.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	/*
		首先尝试从CycleState中获取NodeNetRequestMap和NodeNetCapacityMap
		如果获取失败，则说明是一个新的集群，需要初始化这两个Map对象
		如果获取成功，则说明不是一个新的集群，不需要初始化这两个Map对象
	*/
	//FIXME:写入CycleState中的数据需要实现Clone方法，因此需要将map封装成一个结构体，然后实现Clone方法(Done)
	//FIXME:获取网络资源的Capacity，这里暂定为100m
	//FIXME:获取已经request的资源用prometheus来实现，因此不需要requestMap
	_, err = state.Read("NodeNetCapacityMap")

	if err != nil {
		capacityMap := NetResourceMap{}
		capacityMap.mmap = make(map[string]float64)
		//初始化NodeNetCapacityMap
		for _, node := range nodeList {
			//capacityMap.mmap[node.Node().Name] = node.Node().Status.Capacity.;
			capacityMap.mmap[node.Node().Name] = 50
		}
		//将NodeNetCapacityMap存储到CycleState中
		state.Write("NodeNetCapacityMap", &capacityMap)
	}

	return framework.NewStatus(framework.Success, "")
}

// PreFilterExtensions ...
func (*CustomScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (n *CustomScheduler) Score(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) (int64, *framework.Status) {
	/*
		基本思路是：
		1.直接利用handler获取节点的CPU、内存信息
		2.网络资源的Capacity从CycleState中获取，而已经申请的网络资源从Promehteus中获取
		3.计算节点运行该Pod后的CPU、内存、网络资源的剩余量
		4.带入公式计算得分
	*/
	klog.V(1).Infof("score pod: %v,current node is %v\n", p.Name, nodeName)
	var Cnet, Cmemory, Ccpu float64
	var Rnet, Rmemory, Rcpu float64
	var Unet, Umemory, Ucpu float64
	//获取节点CPU、内存、网络资源的Capacity
	node, err := n.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, err.Error())
	}

	capacityMap, err := state.Read("NodeNetCapacityMap")
	if err != nil {
		return 0, framework.NewStatus(framework.Error, err.Error())
	}
	Cnet = capacityMap.(*NetResourceMap).mmap[nodeName]
	Ccpu = n.cpuCoreNum[nodeName] * 1000
	Cmemory = float64(node.Node().Status.Capacity.Memory().Value())

	//获取节点CPU、内存、网络资源的已经被使用的数目
	//klog.V(1).Infof("node %v 的CPU资源总量为%v,其中可用的为%v\n", nodeName, node.Node().Status.Capacity.Cpu().String(), node.Node().Status.Allocatable.Cpu().String())
	//cpuAllocatable := float64(node.Node().Status.Allocatable.Cpu().Value())
	memoryAllocatable := float64(node.Node().Status.Allocatable.Memory().Value())

	Ucpu = util.QueryCpuUsageByNode(nodeName) * Ccpu //注意这里prometheus查询的是CPU的占用率，而不是CPU的使用量
	Umemory = Cmemory - memoryAllocatable
	Unet = util.QueryNetUsageByNode(nodeName)

	//获取当前pod的CPU、内存、网络资源的申请数目
	//FIXME:各个pod的CPU、内存请求数目获取似乎有问题,参考这里：https://github.com/prodanlabs/scheduler-framework/blob/main/pkg/plugins.go 66行
	containerNum := len(p.Spec.Containers)
	Rcpu = 0
	Rmemory = 0
	Rnet = 0
	for i := 0; i < containerNum; i++ {
		klog.V(1).Infof("pod %v 的 container %v 获取到的原始（转为float之前） cpu request 是 %v\n", p.Name, i, p.Spec.Containers[i].Resources.Requests.Cpu().String())
		klog.V(1).Infof("pod %v 的 container %v 获取到的原始（转为float之前） memory request 是 %v\n", p.Name, i, p.Spec.Containers[i].Resources.Requests.Memory().Value())
		cpuRequest, _ := strconv.ParseFloat(strings.Replace(p.Spec.Containers[i].Resources.Requests.Cpu().String(), "m", "", 1), 64)
		klog.V(1).Infof("pod %v 的 container %v 获取到的转为float之后的 cpu request 是 %v\n", p.Name, i, cpuRequest)
		Rcpu += cpuRequest
		Rmemory += float64(p.Spec.Containers[i].Resources.Requests.Memory().Value())
		net, err := strconv.ParseFloat((p.Labels["netRequest"]), 64)
		if err != nil {
			return 0, framework.NewStatus(framework.Error, err.Error())
		}
		Rnet += net
	}

	//计算节点运行该Pod后的CPU、内存、网络资源的使用量
	// Tcpu = Ucpu + Rcpu
	// Tmemory = Umemory + Rmemory
	// Tnet = Unet + Rnet

	//计算节点运行该Pod后的CPU、内存、网络资源的剩余量
	// ECpu := Ccpu - Tcpu
	// Ememory := Cmemory - Tmemory
	// Enet := Cnet - Tnet
	//输出归一化之前的C、R、U
	klog.V(1).Infof("现在输出归一化之前的C、R、U\n")
	klog.V(1).Infof("Ccpu: %v,Cmemory: %v,Cnet: %v\n", Ccpu, Cmemory, Cnet)
	klog.V(1).Infof("Rcpu: %v,Rmemory: %v,Rnet: %v\n", Rcpu, Rmemory, Rnet)
	klog.V(1).Infof("Ucpu: %v,Umemory: %v,Unet: %v\n", Ucpu, Umemory, Unet)

	//TODO：将C、R、U、T归一化到[0,100]之间(Done)
	Rcpu = Rcpu / Ccpu * 100
	Rmemory = Rmemory / Cmemory * 100
	Rnet = Rnet / Cnet * 100
	Ucpu = Ucpu / Ccpu * 100
	Umemory = Umemory / Cmemory * 100
	Unet = Unet / Cnet * 100
	Ccpu = 100
	Cmemory = 100
	Cnet = 100

	//计算资源平均使用数
	U := (Ucpu + Umemory + Unet) / 3

	//将C、R、U输出到日志中
	klog.V(1).Infof("现在输出归一化之后的C、R、U\n")
	klog.V(1).Infof("Ccpu: %v,Cmemory: %v,Cnet: %v\n", Ccpu, Cmemory, Cnet)
	klog.V(1).Infof("Rcpu: %v,Rmemory: %v,Rnet: %v\n", Rcpu, Rmemory, Rnet)
	klog.V(1).Infof("Ucpu: %v,Umemory: %v,Unet: %v\n", Ucpu, Umemory, Unet)
	// klog.V(1).Infof("Tcpu: %v,Tmemory: %v,Tnet: %v\n", Tcpu, Tmemory, Tnet)

	//带入公式计算得分
	scorePart1 := (1 / (n.resourceToWeightMap["cpu"] + n.resourceToWeightMap["memory"] + n.resourceToWeightMap["net"])) *
		(3 - (Rcpu*n.resourceToWeightMap["cpu"]/(Ccpu-Ucpu) + Rmemory*n.resourceToWeightMap["memory"]/(Cmemory-Umemory) + Rnet*float64(n.resourceToWeightMap["net"])/(Cnet-Unet)))
	scorePart2 := (Ucpu-U)*(Ucpu-U) + (Umemory-U)*(Umemory-U) + (Unet-U)*(Unet-U)
	finalScore := scorePart1 - scorePart2/3

	klog.V(1).Infof("score pod: %v,current node is %v,final score is %v\n", p.Name, nodeName, finalScore)

	finalScore *= 100000000
	klog.V(1).Infof("score pod: %v,current node is %v,final score is %v\n", p.Name, nodeName, finalScore)
	return int64(finalScore), framework.NewStatus(framework.Success, "")
}

func (pl *CustomScheduler) NormalizeScore(ctx context.Context, state *framework.CycleState, p *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	var (
		highest int64 = 0
		lowest        = scores[0].Score
	)
	klog.Infoln("--------->", scores)
	for _, nodeScore := range scores {
		klog.Infoln("highest for:--------->", highest)
		klog.Infoln("lowest for:--------->", lowest)
		if nodeScore.Score < lowest {
			lowest = nodeScore.Score
		}
		if nodeScore.Score > highest {
			highest = nodeScore.Score
		}
	}
	klog.Infoln("highest:--------->", highest)
	klog.Infoln("lowest:--------->", lowest)
	if highest == lowest {
		lowest--
	}

	for i, nodeScore := range scores {
		scores[i].Score = (nodeScore.Score - lowest) * framework.MaxNodeScore / (highest - lowest)
		klog.Infof("node: %v, 经过归一化后的 final Score为: %v", scores[i].Name, scores[i].Score)
	}
	return framework.NewStatus(framework.Success, "")
}

func (n *CustomScheduler) ScoreExtensions() framework.ScoreExtensions {
	return n
}

// New ... Create an scheduler instance
// New() is type PluginFactory = func(configuration runtime.Object, f v1alpha1.FrameworkHandle) (v1alpha1.Plugin, error)
// mentioned in https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/framework/runtime/registry.go
func New(_ *runtime.Unknown, handle framework.FrameworkHandle) (framework.Plugin, error) {
	plugin := &CustomScheduler{}
	plugin.handle = handle
	plugin.resourceToWeightMap = map[string]float64{
		"cpu":    1,
		"memory": 1,
		"net":    1,
	}
	plugin.cpuCoreNum = map[string]float64{
		"master": 8,
		"node1":  8,
		"node2":  10,
		"node3":  12,
	}
	klog.Info("CustomScheduler plugin is created!\n")
	return plugin, nil
}
