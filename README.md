# work-cli

A small local time-tracking CLI.

## Install

```bash
make install
```

Installs the CLI as `work` into `$(go env GOPATH)/bin` by default.
Set `BINDIR` to install elsewhere.

To install the latest GitHub release:

```bash
curl -fsSL https://github.com/Rasalas/work-cli/releases/latest/download/install.sh | bash
```

The installed binary can manage release installs:

```bash
work update
work uninstall
```

## Release

Push a semver tag to build and publish release binaries:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

The release workflow uploads Linux, macOS, and Windows binaries plus SHA-256
checksums directly to the GitHub release. It does not retain separate Actions
artifacts or dependency caches.

## Usage

```bash
work start 800 -p someproject
work do "parser debuggen"
work doing "sqlite migration pruefen"
work done "migration laeuft"
work done --last "Feiertagssupport wegen Produktionsfix"
work doing --session 1 --at start "Feiertag, gearbeitet wegen Release"
work status
work end 1402
work log --today
work log --date 2026-05-25
work edit 1 --end 1430
work db path
```

Data is stored in SQLite at `~/.local/share/work-cli/work.sqlite` by default.
Set `WORK_DB` to use another path.

## Projects

```bash
work project add someproject
work project list
work start 800
```

When multiple active projects exist and no `-p/--project` is given,
`work start` opens an interactive project picker.
