cd integration/benchmark

# benchmark test
#sudo "PATH=$PATH" env go test -bench="BenchmarkWasmModuleInCri" -count=3 . > bench.log
#sudo "PATH=$PATH" env go test -bench="BenchmarkImageInCri" -count=3 . >> bench.log

sudo "PATH=$PATH" env go test -bench="BenchmarkWasmInstanceInCri" -runtime-handler=wasm . > ../../bench.log
sudo "PATH=$PATH" env go test -bench="BenchmarkContainerInCri" . >> ../../bench.log

# return to root
cd ..
