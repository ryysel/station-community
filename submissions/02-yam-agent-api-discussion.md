# DRAFT — not submitted

Target: https://github.com/nyakokitsu/yam/issues
Proposed title: Text-IPC voice-agent package: request correlation and resource ownership

We are preparing a separate native Go agent as a YAM package proposal. It consumes
stock Quasar ASR text, requests an explicitly configured cloud LLM, and speaks via
the existing audio route. It does not capture the microphone or replace stock ASR.

Before implementing a production adapter, we would like to align on:

- Stable speech request IDs and response/source correlation, so only the station
  receiving the stock cloud response answers. A priority fallback is not proximity.
- Narrow permissions for cancel/continue of the selected request, without exposing
  arbitrary IPC protobuf/directives to packages.
- Ownership/handoff of audio and indicators between stock Alice, AirPlay and agents.
- Lifecycle cancellation and child audio cleanup when YAM stops a package.
- Provider credentials outside packages; no raw speech in ordinary diagnostic logs.

We can contribute source/tests and separate safe fixtures. Our current source
export is not an accepted .yap; YAM has only passed read-only and silent-volume
smoke tests on our models. Which existing or planned APIs should an agent use?
