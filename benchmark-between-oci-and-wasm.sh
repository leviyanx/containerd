cd integration
# benchmark test
sudo "PATH=$PATH" env go test -bench="BenchmarkWasmModuleInCri" -benchtime=10x .

# return to root
cd ..
