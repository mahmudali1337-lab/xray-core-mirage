# xray-core-mirage

Fork of [XTLS/Xray-core](https://github.com/XTLS/Xray-core) that adds **MIRAGE** вЂ”
a new VLESS flow `xtls-rprx-mirage` designed as a successor to `xtls-rprx-vision`.

## What is MIRAGE

MIRAGE is a length-obfuscated, AEAD-framed inner protocol that runs inside a
VLESS+REALITY tunnel. Unlike Vision, which leaks raw TLS record sizes after the
handshake, MIRAGE wraps every payload in random-padded encrypted frames so the
on-wire traffic profile no longer matches plain TLS proxying.

Highlights:

- New flow id: `xtls-rprx-mirage`
- Framing: `MIRAGE/v1/c2s` and `MIRAGE/v1/s2c` AEAD streams with random padding
- Drop-in replacement for `xtls-rprx-vision` in any existing VLESS+REALITY inbound
- Works with the standard REALITY handshake (no protocol changes there)
- Fully compatible with the rest of xray-core (routing, sniffing, dispatcher, etc.)

## Server config

Replace `flow` in your VLESS inbound:

```jsonc
{
  "tag": "VLESS_MIRAGE",
  "port": 443,
  "listen": "0.0.0.0",
  "protocol": "vless",
  "settings": {
    "flow": "xtls-rprx-mirage",
    "clients": [
      { "id": "<uuid>", "flow": "xtls-rprx-mirage" }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "raw",
    "security": "reality",
    "realitySettings": { /* same as Vision */ }
  }
}
```

## Client URL

```
vless://<uuid>@host:443?encryption=none&flow=xtls-rprx-mirage&type=tcp&security=reality&sni=...&fp=chrome&pbk=...&sid=...
```

A MIRAGE-aware client is required. The reference client implementation lives in
the matching mihomo fork: [mahmudali1337-lab/mihomo-mirage](https://github.com/mahmudali1337-lab/mihomo-mirage).

## Build

```bash
go build -o xray ./main
```

Produces a single static binary that exposes both `xtls-rprx-vision` and
`xtls-rprx-mirage` flows.

A multi-arch Docker image is built by CI and used by the matching
[remnanode-mirage](https://github.com/mahmudali1337-lab/remnanode-mirage) image.

## Status

Production-tested through Remnawave panel + Remnanode + custom client. UDP is
not supported (same restriction as Vision). For everything not related to
MIRAGE, see the [upstream Xray-core](https://github.com/XTLS/Xray-core).

## License

[MPL-2.0](LICENSE) — same as upstream.
