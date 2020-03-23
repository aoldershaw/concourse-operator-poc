# concourse-operator-poc

## Getting Started

```sh
# Install CRD
make install
# Create a "Concourse" instance
kubectl apply -f config/samples/deploy_v1alpha1_concourse.yaml
# Run controller locally
make run
```

## Prerequisites

* K8s v1.16+ - note that this can easily be changed by not using [Server Side Apply], but it was more convenient to use it.

[Server Side Apply]: https://kubernetes.io/docs/reference/using-api/api-concepts/#server-side-apply
