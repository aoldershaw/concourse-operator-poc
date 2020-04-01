# concourse-operator-poc

## Getting Started

```sh
# Install CRD
make install
# Create a "Concourse" instance
kubectl apply -f config/samples/deploy_v1alpha1_concourse.yaml
# Temporary hack: grant Role to default service account
kubectl apply -f config/hack/rbac.yaml
# Run controller locally
make run
```

Note: with the RBAC hack, this needs to be deployed in the default namespace.

## Prerequisites

* K8s v1.16+ - note that this can easily be changed by not using [Server Side Apply], but it was more convenient to use it.

[Server Side Apply]: https://kubernetes.io/docs/reference/using-api/api-concepts/#server-side-apply
