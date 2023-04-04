# test
cd integration
sudo "PATH=$PATH" env go test -v -run "TestWasmModuleInCri" . -test.v
sudo "PATH=$PATH" env go test -v -run "TestWasmInstanceRestart" -runtime-handler=wasm . -test.v

# return to root
cd ..
