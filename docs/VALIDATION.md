# Preparation validation — 2026-10-25

This report applies to the clean source-review export, not device deployment.

Passed locally in the preparation environment:

- Python source audit and five audit unit tests, including a secret-like fixture
  removed from a synthetic repository's worktree but still caught in Git history.
- Exact-match comparison against the deployment peer key, held outside this tree.
  No matching value is included in the export; the value is never printed.
- Agent: Go1.27.3, go test -count=1 ./..., go vet ./..., go test -race -count=1 ./....
- Agent: CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 build.
- C: native build and unit tests with -Wall -Wextra -Werror, libdbus/OpenSSL headers.
- C: ARM hard-float cross-build with the existing isolated compiler/sysroot.

The modified source has NOT been installed on speakers. No provider requests,
appliance IPC, audio playback, hardware LEDs or autostart changes were used in
these preparation tests. Downloading compiler/module dependencies is distinct
from contacting inference/TTS services. Build artifacts remain outside this tree.

Review gates still open: owner public identity/destination approval, maintainer
API agreement, real YAM lifecycle adapter, target-device regression/cold-boot tests,
dependency notices for any eventual binary distribution and registry acceptance.
The audit catches listed signatures/private patterns; it is not proof that every
possible secret or security issue is absent. Manual review is still required.
