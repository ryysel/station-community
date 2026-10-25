# Station Community — source-review preparation

Original source contributions for collaboration with the Yandex Station YAM
community. **Not a production installer or a ready registry release.**

## Contents

- `agent/`: native Go text-IPC voice-agent bridge and unit tests. Uses the stock
  speech recognition path; microphone capture is not implemented by this bridge.
- `airplay/group-volume/`: standalone source proposal for authenticated reverse
  volume propagation scoped to a receiver's own live AirPlay2 group.
- `submissions/`: unsubmitted issue/PR drafts for maintainers.
- `docs/`: architecture, privacy, limitations, release gates and contribution map.
- `tools/audit_export.py`: source/history checks for accidental private material.

There are **no private repository objects/history, deployment secrets, appliance
snapshots, voice logs, vendor binaries or compiled release packages** here.
Documentation addresses are non-routable test fixtures, not an inventory.

## Tests (no devices or sound)

```sh
python3 tools/audit_export.py
cd agent
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o /tmp/station-agent-arm .
cd ../airplay/group-volume
make test
```

Go1.23+; C tests require a compiler, pkg-config, libdbus-1 development files and
OpenSSL development files. Unit tests use mocks/fixtures and do not execute the
agent main loop, contact providers, access appliance IPC or play audio.
Cross-compiled output is a test artifact, not a certified release.

## Status

The source derives from a working private appliance integration, but this export
has deliberate portability/privacy changes. It has **not** been installed on the
devices. Historical user reports of four-speaker playback/voice group volume do
not prove this modified tree's cold boot, long-term stability or YAM integration.

There is no second Shairport implementation bundled: prefer contributing native
audio profiles and reverse/group-volume support to the existing
[Station-airplay](https://github.com/Yandex-Station-Universal-Repo/Station-airplay).
The agent is a separate proposal, not part of the audio receiver package.

See [review and release gates](docs/RELEASE_CHECKLIST.md) before any publication.
MIT applies to original source only; see [third-party notice](docs/THIRD_PARTY.md).
