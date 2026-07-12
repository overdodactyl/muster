# Contributing

Thanks for considering a contribution.

## Development

```bash
git clone https://github.com/overdodactyl/muster.git
cd muster
make test          # run the test suite
make build         # build ./bin/muster
make install       # install to $PREFIX/bin (default: ~/.local/bin)
```

Requires Go 1.24+ and a POSIX shell.

## Style

- Format with `go fmt ./...` before committing.
- Vet with `go vet ./...`.
- Use conventional commits (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`,
  `test:`, `ci:`) with a scope where sensible, e.g. `feat(dash): add sparkline`.

## Pull requests

- Keep PRs focused. One logical change per PR.
- Include a short description of what changed and why.
- CI must pass (build, tests, vet, gofmt).

## Reporting bugs

Open an issue with:

- Slurm version (`sinfo -V`)
- muster version (`muster version`)
- Command you ran and what you expected to happen
- Actual output, redacted of anything sensitive

Please do not paste real cluster hostnames, usernames, or job details you
consider private into public issues.
