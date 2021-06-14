# Mock Clients

Run the following command to generate mock clients:

```sh
mockgen -source=$GOPATH/src/github.com/Azure/azure-container-networking/cns/cnsclient/apiclient.go -package=mockclients APIClient >cnsclient.go
mockgen -source=$GOPATH/src/sigs.k8s.io/controller-runtime/pkg/client/interfaces.go -package=mockclients Client >kubeclient.go
```
