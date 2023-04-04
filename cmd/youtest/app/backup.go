package app

// const (
//   Ns = "youtestdefault"
//   Id = "youtestid"
//   Base = "/home/youtirsin3/workspace/bundletest/new"
//   Root = "/home/youtirsin3/workspace/bundletest/root"
//   State = "/home/youtirsin3/workspace/bundletest/state"
//   Address = ""
//   TTRPCAddress = ""
// 	SchedCore = true
// )

// func prepareShimManager() (*v2.ShimManager, error) {
//   ctx:= namespaces.WithNamespace(context.Background(), Ns)
//
//   config := v2.ManagerConfig {
//     Root: Root,
//     State:State,
//     Address: Address,
//     TTRPCAddress: TTRPCAddress,
//     Events: nil,
//     SchedCore: SchedCore,
//   }
//
//   shim, err := v2.NewShimManager(ctx, &config)
//   if err != nil {
//     return nil, err
//   }
//   return shim, nil
// }

// func prepareBundle() (*v2.Bundle, error) {
//   ctx:= namespaces.WithNamespace(context.Background(), Ns)
//
//   spec, err := prepareSpec()
//   if err != nil {
//     return nil, err
//   }
//
//   bundle, err := v2.NewBundle(ctx, Root, State, Id, spec)
// 	if err != nil {
//     return nil, err
// 	}
//   return bundle, nil
// }

// func prepareSpec() ([]byte, error) {
//   spec, err := ioutil.ReadFile("/home/youtirsin3/workspace/bundletest/mycontainer/config.json")
//   if err != nil {
//     fmt.Println("err reading spec")
//     return nil, err
//   }
//   return spec, nil
// }
