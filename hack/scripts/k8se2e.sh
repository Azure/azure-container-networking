echo "Cluster -g $GROUP -n $CLUSTER"

eval k8sVersion="v"$( az aks show -g $GROUP -n $CLUSTER --query "currentKubernetesVersion")
echo $k8sVersion "e2e Test Suite"

curl -L https://dl.k8s.io/$k8sVersion/kubernetes-test-linux-amd64.tar.gz -o ./kubernetes-test-linux-amd64.tar.gz

tar -xvzf kubernetes-test-linux-amd64.tar.gz --strip-components=3 kubernetes/test/bin/ginkgo kubernetes/test/bin/e2e.test
