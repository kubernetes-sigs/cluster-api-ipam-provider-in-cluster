# Cluster API IPAM Provider In Cluster

This is a IPAM provider for Cluster API that manages pools of IP addresses using Kubernetes resources. It serves as a reference implementation for IPAM providers, but can also be used as a simple replacement for DHCP.

## Usage

This provider comes with a `InClusterIPPool` resource to specify the pools from which addresses should be assigned. You can provide an address range using start and end addresses, as well as a prefix length, or a set of addresses with the prefix and gateway.

```yaml
apiVersion: ipam.cluster.x-k8s.io/v1alpha2
kind: InClusterIPPool
metadata:
  name: inclusterippool-sample
spec:
  addresses:
    - 10.0.0.10
    - 10.0.0.24-10.0.0.32
    - 10.0.0.128/28
  prefix: 24
  gateway: 10.0.0.1
```

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](https://slack.k8s.io/)
- [Mailing List](https://groups.google.com/a/kubernetes.io/g/dev)

## Licensing

Copyright (c) 2022 Deutsche Telekom AG.

Licensed under the **Apache License, Version 2.0** (the "License"); you may not use this file except in compliance with the License.

You may obtain a copy of the License at https://www.apache.org/licenses/LICENSE-2.0.

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the [LICENSE](./LICENSE) for the specific language governing permissions and limitations under the License.

### Dependency Licenses

You can find the licenses of used Go dependencies as a `licenses.tar.gz` archive as part of our [releases](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/releases) and in the `/license` directory contained in our container images available at [ghcr.io/telekom/cluster-api-ipam-provider-in-cluster](https://ghcr.io/telekom/cluster-api-ipam-provider-in-cluster).
