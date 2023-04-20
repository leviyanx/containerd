# clean last build
make clean

# install containerd
make GO_BUILDTAGS="no_btrfs"

# start containerd
./bin/containerd &
