# Cluster API IPAM Provider In Cluster

This is an [IPAM provider](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220125-ipam-integration.md#ipam-provider) for [Cluster API](https://github.com/kubernetes-sigs/cluster-api) that manages pools of IP addresses using Kubernetes resources. It serves as a reference implementation for IPAM providers but can also be used as a simple replacement for DHCP.

IPAM providers allow control over how IP addresses are assigned to Cluster API Machines. It is usually only useful for non-cloud deployments. The infrastructure provider in use must support IPAM providers to use this provider.

## Features

- Manages IP Addresses in-cluster using custom Kubernetes resources
- Address pools can be cluster-wide or namespaced
- Pools can consist of subnets, arbitrary address ranges and/or individual addresses
- Both IPv4 and IPv6 are supported
- Individual addresses, ranges and subnets can be excluded from a pool
- Well-known reserved addresses are excluded by default, which can be configured per pool

## Setup via clusterctl

This provider comes with clusterctl support and can be installed using `clusterctl init --ipam in-cluster`.

## Usage

This provider comes with two resources to specify pools from which addresses can be allocated: the `InClusterIPPool` and the `GlobalInClusterIPPool`. As the names suggest, the former is namespaced, the latter is cluster-wide. Otherwise they are identical. The following examples will all use the `InClusterIPPool`, but all examples work with the `GlobalInClusterIPPool` as well.

A simple pool that covers an entire /24 IPv4 network could look like this:

```yaml
apiVersion: ipam.cluster.x-k8s.io/v1alpha2
kind: InClusterIPPool
metadata:
  name: inclusterippool-sample
spec:
  addresses:
    - 10.0.0.0/24
  prefix: 24
  gateway: 10.0.0.1
```

IPv6 is also supported, but a single pool can only consist of v4 **or** v6 addresses, not both. For simplicity we'll stick to IPv4 in the examples.

The `addresses` field supports CIDR notation, as well as arbitrary ranges and individual addresses. Using the `excludedAddresses` field, addresses, ranges or subnets can be excluded from the pool.

```yaml
apiVersion: ipam.cluster.x-k8s.io/v1alpha2
kind: InClusterIPPool
metadata:
  name: inclusterippool-sample
spec:
  addresses:
    - 10.0.0.0/24
    - 10.0.1.10-10.0.1.100
    - 10.0.2.1
    - 10.0.2.2
  excludeAddresses:
    - 10.10.0.16/28
    - 10.10.0.242
    - 10.0.1.25-10.0.1.30
  prefix: 22
  gateway: 10.0.0.1
```

Be aware that the prefix needs to cover all addresses that are part of the pool. The first network in the `addresses` list and the `prefix` field, which specifies the length of the prefix, is used to determine the prefix. In this case, `10.1.0.0/24` in the `addresses` list would lead to a validation error.

The `gateway` will never be allocated. By default, addresses that are usually reserved will not be allocated either. For v4 networks this is the first (network) and last (broadcast) address within the prefix. In the example above that would be `10.0.0.0` and `10.0.3.255` (the latter not being in the network anyway). For v6 networks the first address is excluded.

If you want to use all networks that are part of the prefix, you can set `allocateReservedIPAddresses` to `true`. In the example below, both `10.0.0.0` and `10.0.0.255` will be allocated. The gateway will still be excluded.

```yaml
apiVersion: ipam.cluster.x-k8s.io/v1alpha2
kind: InClusterIPPool
metadata:
  name: inclusterippool-sample
spec:
  addresses:
    - 10.0.0.0/24
  prefix: 24
  gateway: 10.0.0.1
  allocateReservedIPAddresses: true
```


### IPv6 Status Limitations

This provider does fully support IPv6 pools, but they come with a small limitation related to the pools' status. Since the IPv6 address space is very large it exceeds the int64 range, making it cumbersome to calculate the total amount of available addresses for large pools. The status will therefore be limited to the maximum int64 value (2^64), even when more addresses are available. Since the Kubernetes api server will probably not allow storing enough resources to exhaust the pool anyway, we've decided to keep this limitation in favour of simpler implementation.

## Community, discussion, contribution, and support

The in-cluster IPAM provider is part of the cluster-api project. Please refer to it's [readme](https://github.com/kubernetes-sigs/cluster-api?tab=readme-ov-file#-community-discussion-contribution-and-support) for information on how to connect with the project.

The best way to reach the maintainers of this sub-project is the [#cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel on the [Kubernetes Slack](https://slack.k8s.io).

Pull Requests and feedback on issues are very welcome! See the [issue tracker](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/issues) if you're unsure where to start, especially the [Good first issue](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22) and [Help wanted](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/issues?utf8=%E2%9C%93&q=is%3Aopen+is%3Aissue+label%3A%22help+wanted%22+) tags, and also feel free to reach out to discuss.

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/blob/main/code-of-conduct.md).
