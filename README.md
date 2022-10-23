> 集成了prometheus的自定义K8S scheduler，主要参考了这里的实现：https://github.com/justin0u0/ouo-scheduler

# 用法

1. 构造docker镜像
  ```
  make image
  ```
2. 更改`deployments/k8s-scheduler.yaml`中的`image`为你构造的镜像
3. 部署
  ```
  kubectl apply -f deployments/k8s-scheduler.yaml
  ```
