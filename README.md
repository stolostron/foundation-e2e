# foundation-e2e
End-to-end tests for the foundation components.

## Running the tests using Docker image

### Build the image
```sh
make build-image
```

### Run test cases of ManifestWork
Make sure you have kubeconfig files in the current directory for both the hub cluster and the managed cluster on which you want to run the test cases.
```sh
docker run -it -v $(pwd):/opt quay.io/stolostron/foundation-e2e:latest /work-e2e --ginkgo.v --ginkgo.label-filter=sanity-check -hub-kubeconfig=/opt/<hub_kubeconfig_file> -cluster-name=<managed_cluster_name> -managed-kubeconfig=/opt/<managed_kubeconfig_file>
```

### Run test cases of Placement
Make sure you have the kubeconfig file of the hub cluster in the current directory.
```sh
docker run -it -v $(pwd):/opt quay.io/stolostron/foundation-e2e:latest /placement-e2e --ginkgo.v --ginkgo.label-filter=sanity-check -hub-kubeconfig=/opt/<hub_kubeconfig_file> -create-global-clusterset=false -tolerate-unreachable-taint
```
