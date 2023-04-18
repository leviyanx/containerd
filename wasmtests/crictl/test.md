# Tests with crictl

## Prerequisites

**wasmedge shim**

- [containerd/runwasi: Facilitates running Wasm / WASI workloads managed by containerd (github.com)](https://github.com/containerd/runwasi)

refer to [runwasi official](https://github.com/containerd/runwasi) for installation of wasmedge shim.

**crictl**

- [kubernetes-sigs/cri-tools: CLI and validation tools for Kubelet Container Runtime Interface (CRI) . (github.com)](https://github.com/kubernetes-sigs/cri-tools)

***issue***

```
FATA[0000] run pod sandbox: rpc error: code = Unknown desc = failed to create containerd task: failed to create shim task: OCI runtime create failed: runc create failed: expected cgroupsPath to be of format "slice:prefix:name" for systemd cgroups, got "/k8s.io/9a586f6684ee8ad93c29343da5813fa996109ee936732e1927bacd970b65ece6" instead: unknown
```

**solution**

set cgroup driver back to cgroupfs from systemd for containerd.

```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
	...
	SystemdCgroup = false
	# SystemdCgroup = true
	...
```

## Tests

```bash
# pwd containerd
cd wasmtests/crictl/
```

### Image

```bash
# pull
crictl pull \
	--annotation wasm.module.url="https://raw.githubusercontent.com/Youtirsin/wasi-demo-apps/main/wasi-demo-app.wasm" \
	--annotation wasm.module.filename="wasi-demo-app.wasm" \
	wasi-demo-app

# list
crictl image

# delete
crictl rmi wasi_example_main
```

### Instance

run a pod (sandbox) first

```yaml
# wasm-sandbox.yaml
metadata:
  name: wasm-sandbox
  namespace: default
  attempt: 1
  uid: "hdishd83djaidwnduwk28bcsb"
log_directory: "/temp"
linux:
```

```bash
# run pod
crictl runp --runtime=wasm wasm-sandbox.yaml

# list pod
crictl pods

# stop pod
crictl stopp [POD-ID]

# delete pod
crictl rmp [POD-ID]
```

run wasm container. *make sure state of pod is ready*

```yaml
metadata:
  name: wasi-demo-app
image:
  image: wasi-demo-app
  annotations:
    wasm.module.url: "https://raw.githubusercontent.com/Youtirsin/wasi-demo-apps/main/wasi-demo-app.wasm"
    wasm.module.filename: "wasi-demo-app.wasm"
command: ["wasi-demo-app.wasm", "daemon"]
log_path: "wasi-demo-app.0.log"
linux:
```

```bash
# create
crictl create [POD-ID] wasm-demo-app.yaml wasm-sandbox.yaml
# list
crictl ps -a
# start
crictl start [CONTAINER-ID]
# stop
crictl stop [CONTAINER-ID]
# delete
crictl rm [CONTAINER-ID]
```

