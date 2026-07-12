# Security

muster shells out to the standard Slurm CLIs and to `ssh` / `nvidia-smi` on
compute nodes. It performs no privileged operations, has no server surface
except the optional Prometheus exporter (`muster exporter`, default bind
`:9836`), and reads no credentials from disk.

## Reporting a vulnerability

If you believe you have found a security issue, please open a private
security advisory on GitHub rather than a public issue:

<https://github.com/overdodactyl/muster/security/advisories/new>

Include reproduction steps, affected versions, and any mitigations you've
identified. A response should arrive within a week.
