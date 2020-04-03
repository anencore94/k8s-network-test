# k8s-network-test
Simple Network Testing Tool

- Testing the pod-network in the k8s cluster by creating several busybox-pod and executing ping each other.

## Prerequisites

## Quick Start
- make binary
  - `cd ./pkg && ginkgo build`
  - then you get the excutable binary named `pkg.test`

## Version
- compatible k8s version : v1.15, v1.16, v1.17
  - since it uses go-client library versioned v1.16