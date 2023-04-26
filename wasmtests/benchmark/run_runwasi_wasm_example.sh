set -e

build_wasm_image() {
  git clone https://github.com/containerd/runwasi.git
  pushd runwasi
  make CONTAINERD_NAMESPACE="k8s.io" load
  popd
  rm -rf runwasi
}

# Build the wasi-demo-app image and import it to containerd.
rustup target add wasm32-wasi
crictl -r unix:///run/containerd/containerd.sock images -v | grep ghcr.io/containerd/runwasi/wasi-demo-app:latest || build_wasm_image

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
      "image": "ghcr.io/containerd/runwasi/wasi-demo-app:latest"
    },
    "log_path":"wasm.log",
    "linux": {
    }
}
EOF

# Run a wasm container
crictl -r unix:///run/containerd/containerd.sock run --runtime="wasm" --no-pull container.json pod.json
rm -f container.json pod.json
