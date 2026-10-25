# Publication and release gates

- [ ] Owner approves GitHub account, repository name, visibility and exact content.
- [ ] Author/contact metadata and MIT copyright notice reviewed by owner.
- [ ] Worktree AND every reachable Git blob pass tools/audit_export.py.
- [ ] Known private peer key checked by exact-match scan outside this repository.
- [ ] Go tests/vet and C tests pass; source adaptations distinguished from device tests.
- [ ] No raw logs, real configs, private history, keys, deployed identities or binaries.
- [ ] Maintainers agree on hardware profiles, volume semantics and integration API.
- [ ] Actual YAM lifecycle/IPC/resource adapter implemented and accepted on devices.
- [ ] Complete dependency licenses/notices and reproducible binary build recorded.
- [ ] A real .yap version is built and tested before registry submission.
- [ ] Cold boot, failure recovery, coexistence and nonmember isolation tested for release.

Draft messages may be sent separately after owner approval; do not label an
unimplemented integration or an untested package as ready for registry release.
Registry PR base is packages, path packages/<id>/<version>.yap; published archive
versions are immutable. The first contact should be a small scoped proposal.
