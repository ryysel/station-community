# DRAFT — not submitted

Target: https://github.com/nyakokitsu/yam/issues
Proposed title: Device capability fixtures for Prune and retail Station2

With an unmodified YAM v0.1.14 daemon and a deliberately disabled LED profile:

- Mini2 identifies as yandexmini_2; device/sensor smoke tests pass.
- Lite2 Plum identifies as yandexplum; device/sensor smoke tests pass.
- Lite2 Prune also identifies as yandexplum.
- Retail Station2 identifies as unknown; device/sensor smoke tests still pass.

These are metadata observations, NOT evidence that a Midi/Plum hardware renderer
is compatible with Prune/Station2. We do not propose simply removing compatibility
guards or renaming the model to enable LEDs.

We would like to add independently reviewed model/capability fixtures and then
separate native hardware adapters. Which sanitized device-tree/sysfs facts are
needed for a minimal identification PR? No raw device dumps/identities are attached.
