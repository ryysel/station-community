# Community source-review repository

- This repository is an allowlisted, independent source export, not a clone of
  any private infrastructure repository. Do not import private Git history.
- Never add real credentials, account/device keys, conversation logs, deployment
  configs, home addresses, device inventories, compiled binaries or raw IPC dumps.
- Test fixtures use synthetic text and RFC5737 documentation addresses only.
- Changes here are not deployed to speakers. Do not install, play audio, trigger
  LEDs, send IPC commands or enable autostart as part of build/review work.
- Do not publish, create forks/issues/PRs or push to an external remote until the
  owner approves the exact destination and reviewed content.
- The agent/helper source is MIT; dependency/upstream licenses remain separate.
- Do not describe drafts, mock tests or source adaptations as device acceptance.
- Run tools/audit_export.py, agent tests/vet and C unit tests before sharing.
