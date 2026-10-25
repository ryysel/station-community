# Architecture and integration boundaries

## Agent

Stock Alice/Quasar ASR → local decoded text/request events → request-correlated
cloud-response arbitration → one agent calls its configured LLM → speech output.
Wake-only fallback uses configured peer priority, **not microphone proximity**.
The stock cloud recognition remains; this is not an offline assistant.

The Go bridge uses a native CLI to interrupt/continue selected conversations and
plays generated speech through the existing default ALSA route. LED implementations
are appliance-specific. A YAM adapter must negotiate ownership rather than start
another independent LED/audio controller. No adapter migration is claimed here.
The original service scripts/init hook are intentionally not exported.

The peer arbitration UDP protocol is not authenticated. Restrict use to trusted
networks; do not expose it to WAN or claim it is a secure arbitration service.

## AirPlay volume helper

Receiver volume callbacks establish the native mixer baseline. A stable local
volume change can create a signed peer event. Each receiver independently checks
its own live Avahi publication: own address, LOCAL|OUR_OWN, active-session flag,
valid group UUID, local playback/duck/listening state, and exact requested group.
No historical logs or shared sender IP are used as group identity.

The helper uses the existing private D-Bus and RemoteControl.SetAirplayVolume;
it does not play audio, replace ALSA, implement AirPlay or subscribe to microphones.
Actual monitoring/lifecycle integration into Station-airplay is still a proposal.
There must be exactly one owner of the receiver, its volume and recovery process.

Limitations: UDP best-effort with no ACK/retry; busy/non-playing peers skip commands;
short wall-clock freshness assumes synchronized clocks; RAM replay cache resets
on restart; simultaneous commands need a clearer ordering/conflict policy.
Hardware mixer percentages are not acoustic loudness and are not necessarily
canonical Quasar station volume. Do not replace a canonical volume adapter blindly.
