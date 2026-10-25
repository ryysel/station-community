# DRAFT — not submitted

Target: https://github.com/Yandex-Station-Universal-Repo/Station-airplay/issues
Proposed title: Native-audio profiles and reverse/group volume contributions

Hello! We would like to collaborate with Station-airplay rather than publish a
competing receiver package. A separate private integration has been used on Mini2,
Lite2 Plum/Prune and retail Station2 with native ALSA, stock Alice and AP1 rollback.
The owner reported successful four-speaker playback and current-group voice volume.
This is a user acceptance report, not a universal stability/cold-boot guarantee.

We can propose small reviewable changes:

1. Model-specific native ALSA/runtime profiles, including the dmix ABI mismatch
   encountered when mixing native and foreign libraries.
2. Discussion of upgrading Shairport to a version supporting reverse volume via
   D-Bus RemoteControl.SetAirplayVolume; dependency changes should be independent.
3. A current-group peer-volume prototype: own live Avahi gid/session validation,
   HMAC peer events, replay/expiry/echo guards and negative unit tests.

The source export deliberately removes deployment addresses/keys and makes the
peer list configurable. That modified tree has not been device-accepted yet.
Known limitations include best-effort UDP, clock freshness and replay state reset.
We have not conclusively explained an earlier NQPTP crash and will not hide it.

We see the existing station-volume integration and would like to agree on
canonical Quasar versus ALSA volume semantics and single ownership of lifecycle,
recovery and indicators before porting code. What extension points/base branch
would you prefer, and which small profile or interface PR should come first?
