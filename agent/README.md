# Native voice-agent bridge — source proposal

Read the root privacy/architecture documents first. This source can cancel native
responses, access local text IPC, request cloud inference, play speech and drive
indicators when explicitly executed. **Do not run it as a silent diagnostic.**

Build/test offline behavior with go test ./... and go vet ./.... Select a provider
and model you can access; config.example.json intentionally does not work unchanged.
The runtime config/usage/log paths are generic, not a home deployment inventory.

Source-export changes: provider key may come from STATION_AGENT_API_KEY; location
defaults removed; weather requires opt-in; ordinary transcript/answer logs redacted;
new files use private modes. These changes are not deployed to the original devices.

The current entry point is standalone. No production YAM service manifest/adapter
is claimed: stock IPC port discovery, request correlation, interruption permission,
resource ownership and stopping child audio need maintainer review before packaging.
