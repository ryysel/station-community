# Privacy and local trust

- Supply your provider key in a private runtime config (0600), or leave api_key
  empty and use STATION_AGENT_API_KEY. Never commit runtime configuration.
- The example deliberately has no location, no peers and no working model/key.
  Weather is off unless explicitly enabled with a city and valid coordinates.
- Recognized questions/prompts go to the chosen LLM; gTTS speech text goes to the
  TTS provider. Stock Alice cloud ASR still applies. API keys/quotas belong to users.
- Ordinary phrase/answer logs are redacted in this export. Explicit
  STATION_AGENT_LOG_CONTENT=1 enables private diagnostic content, bounded in length.
  Raw IPC dump mode remains explicitly sensitive; never attach dumps publicly.
  Even redacted logs can contain peer metadata or errors: review before sharing.
- YAM management authentication is outside this code. Keep management on loopback
  or behind real access controls; runtime tokens do not secure management routes.
- The AirPlay HMAC key is generated privately per trusted group, not bundled.
  A shared key trusts all enrolled peers; it is not isolation from root/local ADB.
- This is a source review, not a security audit or a promise of silence while a
  user executes agent/audio main modes. Running the agent can speak and change LEDs.
