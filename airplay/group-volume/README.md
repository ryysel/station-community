# Authenticated current-AirPlay-group volume — source proposal

No receiver/vendor binaries or firmware are bundled. The helper expects an existing
Shairport version supporting RemoteControl.SetAirplayVolume, a private D-Bus/Avahi
bus and the existing receiver callback/playback guard conventions under /data/airplay2.
It is not a complete .yap and is not automatically installed.

Configuration (private, outside Git):

- etc/device.ip: this receiver's address, also present in the peer allowlist;
- etc/group-volume.peers: one explicit unicast IPv4 address per line, up to16;
- etc/group-volume.name: exact own AirPlay mDNS service instance name;
- etc/group-volume.key: a privately generated256-bit key encoded as64hex characters,
  file0600, shared only with explicitly enrolled receivers in the trusted group.

No real addresses/keys are shipped. The .example file uses RFC5737 TEST-NET data.
Source-export peer configuration differs from the original fixed-inventory helper;
it needs its own device acceptance before deployment. Do not overwrite a working
receiver installation with this source-review tree.

make test runs in-memory validation tests only, without sockets/audio/D-Bus access.
Use an architecture-appropriate compiler/sysroot for a deployed helper. Bindings,
library/runtime selection and lifecycle must be integrated with the target package,
not guessed by an automatic installer. See root architecture for known limitations.
