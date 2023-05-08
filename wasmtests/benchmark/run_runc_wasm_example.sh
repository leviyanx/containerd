set -e

# pull wasi-demo-app image
crictl -r unix:///run/containerd/containerd.sock images -v | grep docker.io/leviyanx/runc-wasm-example:wasi-demo-app || crictl -r unix:///run/containerd/containerd.sock pull leviyanx/runc-wasm-example:wasi-demo-app

# Prepare for the wasm Pod and Container config file.
touch pod.json container.json
current_timestamp=$(date +%s)
cat > pod.json <<EOF
{
    "metadata": {
        "name": "test-sandbox$current_timestamp",
        "namespace": "default"
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
      "image": "docker.io/leviyanx/runc-wasm-example:wasi-demo-app"
    },
    "log_path":"wasm.log",
    "linux": {
    }
}
EOF

# Run a wasm container
crictl -r unix:///run/containerd/containerd.sock run --runtime="runc" --no-pull container.json pod.json
rm -f container.json pod.json

