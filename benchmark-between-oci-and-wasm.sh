cd integration/benchmark

# benchmark test
sudo "PATH=$PATH" env go test -bench="BenchmarkWasmModuleInCri" -count=3 . > bench.log

# return to root
cd ..
