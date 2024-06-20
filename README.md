# podinfo Kubernetes Controller 

[![CI (Tests and Lint)](https://github.com/moshevayner/k8s-controller-go-podinfo/actions/workflows/ci_test_lint.yaml/badge.svg)](https://github.com/moshevayner/k8s-controller-go-podinfo/actions/workflows/ci_test_lint.yaml)

## Overview

A Sample k8s Controller built in Golang which implements the [podinfo](https://github.com/stefanprodan/podinfo) web application using a custom resource.

This controller was built using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) framework.

The controller watches for custom resources of type `PodinfoInstance` in the `podinfo-app.podinfo.vayner.me/v1` api group, and creates at the very least a deployment and service for each instance. The controller also watches for changes to the custom resource and updates the deployment and service as needed. Any changes made to the custom resource will be reflected in the underlying components. For example, if you change the `replicaCount` field in the custom resource, the app deployment will be scaled accordingly. There is a minimal set of supported configurations the `podinfo` app currently, but more can be added as needed.

Note that when `redis.enabled` is `true`, the controller will also create a redis deployment and service for each instance.

All of the underlying components will be created in the same namespace as the custom resource, and will have `ownerReferences` set to the custom resource. This means that when the custom resource is deleted, all of the underlying components will be deleted as well.

Refer to the [PodinfoInstance](./api/v1/podinfoinstance_types.go) type or the example [yaml file](./config/samples/podinfo-app_v1_podinfoinstance.yaml) for more details on supported fields.

## Basic Requirements

- An available Kubernetes Cluster, with cluster-level privileges to deploy a CRD at the very least. Preferably- cluster admin is ideal. You may also set up a local cluster (e.g. `minikube`, `kind`, instructions [here](https://kubernetes.io/docs/tasks/tools/)) for testing purposes.
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed and configured to access the cluster.

## Installation

- Run `make deploy` to install the CRD into the cluster. Note: The image used in the deployment is hosted on a public Docker hub repo as it appears in the top of the [Makefile](./Makefile). If you wish to use your own image, please follow the instructions in the [Image Build & Push](#image-build--push) section below.

## Image Build & Push

- Run `make docker-build docker-push` to build and push the image to your registry of choice. You may also set the `IMG` variable to your desired image name and tag, e.g. `IMG=quay.io/username/k8s-controller-go-podinfo:v0.0.1 make docker-build docker-push`

## Running locally

If you wish to run the controller locally, you may do so by running `make install run`. This will install the CRD into your local cluster and run the controller locally, printing the controller log into the stdout of your current shell. Note: You will need to have `kubectl` configured to access your local cluster. **MAKE SURE YOU ARE POINTING TO THE CORRECT CLUSTER BEFORE RUNNING THIS COMMAND.**

You may also compile and build the binary by running `make build` and then run the binary directly by running `./bin/manager`. Note that you will need to have `kubectl` configured to access your local cluster. Please refer to the [Makefile](./Makefile) for more details and the full list of supported targets.

If you need to run the controller with debug logging, you may run `make run-debug`. This would increase the verbosity to level 5 and will output additional logging.

## Notes / Caveats

- After making any change to the controller types (i.e. [this](./api/v1/podinfoinstance_types.go)), you will need to run `make generate manifests` to update the CRD manifest. You will also need to run `make install` to apply the updated CRD into the cluster.

- From a networking standpoint, this project currently doesn't include any Ingress configurations or any other networking components. This means that the app will only be accessible from within the cluster. You may use `kubectl port-forward` to access the app locally. For example, if you have a podinfo instance named `podinfoinstance-sample` in the `default` namespace, you may run `kubectl port-forward svc/podinfoinstance-sample 9898:9898` to access the app locally at `http://localhost:9898`.

## Running tests

Currently there's a minimal set of tests (due to this project being a POC. As it grows, I may add more tests as needed for better coverage) that can be run using `make test`.
This test suite also runs as part of the CI pipeline (see [this](.github/workflows/ci_test_lint.yaml) for reference).

## Example Custom Resource

See [this](./config/samples/podinfo-app_v1_podinfoinstance.yaml) for an example custom resource.
