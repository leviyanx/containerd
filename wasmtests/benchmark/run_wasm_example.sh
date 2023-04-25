set -e

pull_wasm_module() {
crictl pull \
	--annotation wasm.module.url="https://raw.githubusercontent.com/Youtirsin/wasi-demo-apps/main/wasi-demo-app.wasm" \
	--annotation wasm.module.filename="wasi-demo-app.wasm" \
	wasi-demo-app
}

crictl -r unix:///run/containerd/containerd.sock images -v | grep wasi-demo-app:latest || pull_wasm_module

# Prepare for the wasm Pod and Container config file.
touch pod.json container.json
current_timestamp=$(date +%s)
cat > pod.json <<EOF
{
    "metadata": {
        "name": "test-sandbox$current_timestamp",
        "namespace": "default",
        "attempt": 1,
        "uid": "hdishd83djaidwnduwk28bcsb"
    },
    "log_directory": "/tmp",
    "linux": {
    }
}
EOF
cat > container.json <<EOF
{
    "metadata": {
        "name": "wasm",
        "namespace": "default"
    },
    "image": {
      "image": "wasi-demo-app",
      "annotations": {
          "wasm.module.url": "https://raw.githubusercontent.com/Youtirsin/wasi-demo-apps/main/wasi-demo-app.wasm",
          "wasm.module.filename": "wasi-demo-app.wasm"
          }
    },
    "command": [
        "wasi-demo-app.wasm", 
        "daemon"
    ],
    "log_path":"wasm.log",
    "linux": {
    }
}
EOF

# Run a wasm container
crictl -r unix:///run/containerd/containerd.sock run --runtime="wasm" --no-pull container.json pod.json
rm -f container.json pod.json
