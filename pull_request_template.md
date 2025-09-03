## Describe your changes

## Jira Ticket Link
Jira: <OSPRH link>

## Checklist before requesting a review
- [ ] I have performed a self-review of my code and confirmed it passes tests
- [ ] Performed `pre-commit run --all`
- [ ] Tested operator image in a test/dev environment. It can be CRC via [install_yamls](https://github.com/openstack-k8s-operators/install_yamls) or a [hotstack](https://github.com/openstack-k8s-operators/hotstack/tree/main) instance (optional)
- [ ] Verified that no failures present in logs(optional):
  - [ ] ironic-operator-build-deploy-kuttl
  - [ ] podified-multinode-ironic-deployment
