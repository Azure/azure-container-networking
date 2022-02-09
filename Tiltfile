allow_k8s_contexts(k8s_context())
default_registry('ttl.sh/nitishm-12390')
docker_build('azure-npm', '.', dockerfile='npm/Dockerfile', build_args = {
	"VERSION": "v1.4.14-101-gf900e319-dirty",
	"NPM_AI_PATH": "github.com/Azure/azure-container-networking/npm.aiMetadata",
	"NPM_AI_ID": "014c22bd-4107-459e-8475-67909e96edcb"
})
# watch_file('npm')
k8s_yaml('npm/deploy/manifests/controller/azure-npm.yaml')
k8s_yaml('npm/deploy/manifests/daemon/azure-npm.yaml', allow_duplicates=True)

