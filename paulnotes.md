* See comments in CRD but I think most of it should move to status.
* Webhooks needs to fidn right pdbwatcher. 
* controller isn't actually checking it was a new eviction that made it get called.
* Love that you have tests :)

* Checking in private keys is bad. Whats the conventioal way to generate a webhook cert? Could have install script generate one on the fly instead. 

* Love that you have a golangci.yaml for github. Is it doing builds and tests or just lint? 
* Should try and get yamls in sinlge directory so you can just kubectl apply that directory and make your installl script smaller. 
   * then your example yamls like example-pdbwatcher can be sepeate demo dir 
   * ah deployments are under config and use kustomize (kubectl apply ++ )  may just want to move 

* seem to have duplicate rbac soem at root and some at config/rbac. Simplify down. Same for cds 

