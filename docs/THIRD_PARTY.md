# Third-party and provenance notice

The owner selected MIT for the original agent and standalone volume-helper source.
No proprietary firmware, vendor libraries, source snapshots or third-party
executables are included in this export.

- Agent dependency: github.com/hajimehoshi/go-mp3 v0.3.4, Apache-2.0. See upstream
  https://github.com/hajimehoshi/go-mp3/tree/v0.3.4 and retain its notices when
  distributing a linked binary. go.mod/go.sum pin the source dependency.
- The C helper links system libdbus and libcrypto. Those libraries retain their
  own licenses; no library binaries are distributed here.
- Shairport Sync, NQPTP, Avahi, FFmpeg and existing Station-airplay/YAM are separate
  projects. Their licenses/source-distribution duties are NOT replaced by MIT.
  The helper expects an existing installation but does not bundle those projects.
- Existing Station-airplay describes its Go adapter as GPL-3.0-or-later. MIT code
  can be proposed for integration, subject to that project's licensing policy.
- Before a binary/.yap release, include required third-party licenses/notices,
  source versions, patches and reproducible build instructions. A source-review
  export does not satisfy all binary redistribution requirements by itself.
