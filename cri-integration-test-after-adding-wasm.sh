# clean last build
make clean

# install containerd
make GO_BUILDTAGS="no_btrfs"

# start containerd
sudo ./bin/containerd &

# test
cd integration
sudo "PATH=$PATH" env go test -v -run "TestWasmModuleInCri" . -test.v

# return to root
cd ..

# stop containerd
sudo systemctl stop containerd