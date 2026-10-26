# Kubernetes 故障排查知识库

> GoOpsAgent RAG 知识库 — 基于阿里云 ACK 官方排障文档与 CNCF 最佳实践整理

---

## Phase 1: 调度问题 (Scheduling Issues)

### Pod 长时间 Pending

**症状**: `kubectl get pods` 显示 STATUS= `Pending`，`kubectl describe pod` Events 包含调度失败信息。

**常见错误信息与解决方案**:

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `no nodes available to schedule pods` | 集群无可用节点 | 检查节点是否 NotReady；检查是否有节点污染(taint)或亲和性规则过于严格 |
| `0/x nodes are available: x Insufficient cpu` | CPU 资源不足 | 降低 workloads 的 requests 配置；扩容节点池；启用 HPA 弹性伸缩 |
| `0/x nodes are available: x Insufficient memory` | 内存资源不足 | 同上；为 Java 应用优化 `-Xmx` 参数 |
| `x node(s) didn't match pod's node affinity/selector` | nodeSelector/nodeAffinity 规则无节点匹配 | `kubectl get node <name> --show-labels` 查看节点标签，修正 affinity 规则 |
| `x node(s) didn't match pod affinity/anti-affinity rules` | Pod 亲和性/反亲和性规则冲突 | `kubectl get pods -A -l <LABEL> -o wide` 查看已有 Pod 分布 |
| `x node(s) had taint {key: value}, that the pod didn't tolerate` | 节点有污点但 Pod 未配置容忍 | 添加对应的 tolerations 或移除节点 taint |

**诊断命令**:
```bash
kubectl describe pod <pod-name> -n <namespace>
kubectl get nodes -o wide
kubectl describe node <node-name> | grep -A5 "Allocated resources"
kubectl get pod <pod-name> -o yaml | grep -A10 affinity
```

---

## Phase 2: 镜像拉取问题 (Image Pull Errors)

### ImagePullBackOff / ErrImagePull

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `ErrImagePull` | 无法从镜像仓库拉取镜像 | 1. 确认镜像地址、Tag 正确 2. 检查 `imagePullSecrets` 是否配置且有效 3. 在节点上 `crictl pull <image>` 测试连通性 |
| `ImagePullBackOff` | 多次重试拉取失败后进入回退 | 同上排查；确认镜像仓库网络可达 |
| `toomanyrequests: You have reached your pull rate limit` | Docker Hub 匿名拉取限流 | 配置 Docker Hub 认证 Secret 或使用私有镜像仓库 |
| `manifest for xxx not found` | 镜像 Tag 不存在或架构不匹配 | 检查 Tag 是否正确，确认镜像支持当前节点架构(arm64/amd64) |

**诊断命令**:
```bash
kubectl describe pod <pod-name> | grep -A10 Events
kubectl get pod <pod-name> -o yaml | grep -A5 imagePullSecrets
# 在节点上测试
crictl pull <image-address>
curl -v https://<registry-host>/v2/
```

---

## Phase 3: 启动失败 (CrashLoopBackOff)

### 容器反复重启

| 症状 | 原因 | 排查步骤 |
|------|------|---------|
| Pod STATUS=`CrashLoopBackOff` | 容器启动后立即退出 | 1. `kubectl logs <pod> --previous` 查看上次崩溃日志 2. `kubectl describe pod <pod>` 查看 Events |
| 日志含 `exit(0)` / `exit code 0` | 容器没有持久进程，正常退出 | 检查 Dockerfile ENTRYPOINT/CMD，确保有前台进程 |
| Events 含 `Liveness probe failed` | 存活探针失败导致重启 | 检查探针路径、端口是否正确；增加 `initialDelaySeconds`；暂时禁用探针后用 `curl` 手动验证 |
| Events 含 `Startup probe failed` | 启动探针失败 | 同上，特别关注 `failureThreshold` 配置 |
| 日志含 `no space left on device` | 磁盘空间不足 | 扩容云盘/清理无用镜像；配置 `imageGCHighThresholdPercent` |
| 日志含 `Address already in use` | 端口冲突 | 检查 `hostNetwork: true` 是否必要；配置 Pod 反亲和性分散到不同节点 |
| Events 含 `container init caused "setenv: invalid argument"` | Secret 值未 Base64 编码 | 重新创建 Secret 确保值经过 Base64 编码 |
| 无 Events 但启动失败 | 资源配置不足（资源超限） | 检查 limits/requests 配置，启用资源画像(resource profiling) |
| `OOMKilled` | 内存超限被杀死 | 见 Phase 4 OOM 排查 |

---

## Phase 4: 运行时问题 (Runtime Issues)

### OOMKilled

| OOM 层级 | 判断方法 | 解决方案 |
|---------|---------|---------|
| OS 级 OOM | `/var/log/messages` 有 killed 进程但无 cgroup 日志 | 增加节点内存；将高内存 Pod 分散到不同节点 |
| cgroup 级 OOM | `/var/log/messages` 含 `Task in /kubepods.slice/xxx killed as a result of limit` | 增加 Pod memory limits，确保实际使用在 limits 80% 以下；启用资源画像 |

**诊断命令**:
```bash
kubectl describe pod <pod-name> | grep -A5 "State"
kubectl logs <pod-name> --previous
kubectl top pod <pod-name> -n <namespace>
# 在节点上
cat /var/log/messages | grep -i oom
```

**预防建议**:
- Java: 优化 `-Xmx`，建议设为 limits 的 75%
- Node.js: 设置 `--max-old-space-size`
- 通用: 配置 HPA + resource profiling 自动推荐 requests/limits

### Pod Running 但 Not Ready

| 症状 | 原因 | 解决方案 |
|------|------|---------|
| Events 含 `Readiness probe failed` | 就绪探针失败 | 检查探针路径和端口；增加 `failureThreshold`；暂时禁用探针后用 `curl` 验证端点 |
| 应用启动慢 | initialDelaySeconds 太短 | 增加初始延迟；为慢启动应用使用 startupProbe |

### Pod Terminating 卡住

| 原因 | 解决方案 |
|------|---------|
| 节点 NotReady | 等待节点恢复后 Pod 自动删除 |
| 配置了 finalizers | `kubectl get pod -o yaml` 查看并排查 finalizer |
| preStop hook 卡住 | 检查 preStop hook 配置是否正确 |
| 容器未响应 SIGTERM | `kubectl delete pod <name> --grace-period=0 --force` 强制删除；检查应用是否正确处理 SIGTERM |
| 配置了 terminationGracePeriodSeconds | 等待优雅关闭时间过后自动删除 |

### Evicted 驱逐

| 原因 | 解决方案 |
|------|---------|
| 节点资源不足(disk pressure/memory pressure/pid pressure) | 清理节点磁盘；增加节点资源；设置合理的 Pod requests/limits |
| 节点磁盘压力大 | 清理无用镜像和容器；配置 image GC 和 container GC 阈值 |

---

## Phase 5: 网络与服务问题 (Network & Service)

### Service 无法访问

| 症状 | 排查步骤 |
|------|---------|
| ClusterIP 无响应 | 1. 确认 Service selector 与 Pod labels 匹配 (`kubectl get endpoints <svc>`) 2. 确认 targetPort 正确 3. 在 Pod 内 `curl <svc>:<port>` 测试 |
| 外部无法访问(LoadBalancer/NodePort) | 1. 确认 external-ip 已分配 2. 防火墙/安全组是否放行 3. `kubectl get svc <name> -o yaml` 确认端口映射 |

### DNS 解析失败

| 症状 | 排查步骤 |
|------|---------|
| 应用报 `name resolution failed` | 1. 检查 CoreDNS Pod 是否 Running 2. `kubectl run -it debug --image=busybox -- nslookup <domain>` 测试 3. 确认 /etc/resolv.conf 中的 nameserver 配置 |
| CoreDNS CrashLoopBackOff | 查看 CoreDNS 日志；检查内存是否充足 |

### NetworkPolicy 阻断

| 症状 | 排查步骤 |
|------|---------|
| Pod 间无法通信 | `kubectl get networkpolicies -A` 查看规则；使用 `--from`/`--to` 逐步排查 |

---

## 通用故障排查 SOP

### 标准排查流程

1. **看状态**: `kubectl get pods -n <ns> -o wide` → 确定 STATUS
2. **看事件**: `kubectl describe pod <name> -n <ns>` → 查看 Events 字段最近的 Warning/Error
3. **看日志**: `kubectl logs <pod> -n <ns> --tail=100` 和 `--previous`
4. **看资源**: `kubectl top pod <name> -n <ns>` → CPU/Memory 使用率
5. **看配置**: `kubectl get pod <name> -o yaml` → 检查 limits/requests、探针、env、volume
6. **看依赖**: 检查 Service、ConfigMap、Secret、PVC 是否正常
7. **交叉对比**: 同类 Pod 是否也有同样问题？节点资源是否饱满？

### 常用 kubectl 命令速查

```bash
# Pod 状态
kubectl get pods -n <ns> --sort-by=.status.startTime
kubectl get pods -n <ns> --field-selector status.phase=Pending

# Pod 详情
kubectl describe pod <name> -n <ns>
kubectl get pod <name> -n <ns> -o yaml
kubectl get pod <name> -n <ns> -o jsonpath='{.status.containerStatuses[*].state}'

# 日志
kubectl logs <pod> -n <ns> --tail=200 --timestamps
kubectl logs <pod> -n <ns> --previous  # 崩溃前日志
kubectl logs <pod> -n <ns> -c <container>  # 指定容器

# 资源
kubectl top pods -n <ns>
kubectl top nodes

# Deployment
kubectl describe deployment <name> -n <ns>
kubectl rollout history deployment/<name> -n <ns>
kubectl rollout undo deployment/<name> -n <ns>  # 回滚

# 事件
kubectl get events -n <ns> --sort-by='.lastTimestamp'

# 节点
kubectl describe node <node-name> | grep -A10 Conditions
kubectl top node <node-name>
```

### 告警阈值参考

| 指标 | 警告阈值 | 严重阈值 |
|------|---------|---------|
| Pod 重启次数(1h) | > 3 | > 10 |
| Pod Pending 时间 | > 5min | > 15min |
| 节点 CPU 使用率 | > 80% | > 95% |
| 节点 Memory 使用率 | > 85% | > 95% |
| 节点 Disk 使用率 | > 80% | > 90% |
| Pod OOMKilled | 任意一次 | 5min 内重复 |
| CrashLoopBackOff | 出现即告警 | 持续 > 10min |
| ImagePullBackOff | 出现即告警 | 持续 > 5min |