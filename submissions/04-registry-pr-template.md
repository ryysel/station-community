# DRAFT TEMPLATE — NOT READY FOR SUBMISSION

Target: https://github.com/nyakokitsu/yapkg
Base branch: packages (not main)
Path: packages/<approved-id>/<new-semver>.yap

Do not open this PR until the actual adapter/package is implemented and tested.

Required contents for the eventual PR description:

- Source repository and immutable tag; license and third-party notices.
- Reproducible build command, toolchain, target architecture/ABI and SHA256.
- Package id/version equal to path and root plugin.toml fields.
- Minimum permissions and why each is necessary; default credentials absent.
- Exact tested hardware/firmware and untested cases, without private inventories.
- Installation, disable/remove/rollback; no duplicate AirPlay/agent supervisors.
- Validation: python3 tools/build_index.py --packages-dir packages --validate.
- Real device acceptance and known limitations, distinct from structural CI.

Published version archives are immutable: new content requires a new version.
