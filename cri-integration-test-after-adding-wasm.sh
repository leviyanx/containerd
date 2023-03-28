# test
cd integration
sudo "PATH=$PATH" env go test -v -run "TestWasmModuleInCri" . -test.v

# return to root
cd ..
