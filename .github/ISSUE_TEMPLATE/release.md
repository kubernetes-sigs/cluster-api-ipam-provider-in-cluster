---
name: Release
about: Release checklist
labels: kind/release

---

## Release

Release vX.X.X

## Checklist

- [ ] Update metadata & clusterctl-settings
- [ ] Update docs, (compatibility table, usage etc) .
- [ ] Create tag.
- [ ] Update the created draft release to include (BREAKING Changes, Important Notes).
- [ ] Promote the [image](https://github.com/kubernetes/k8s.io/blob/main/registry.k8s.io/images/k8s-staging-capi-ipam-ic/images.yaml), 
- [ ] Publish the release.


/assign @cluster-api-ipam-provider-in-cluster-maintainers
/kind release
