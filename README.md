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
work ls
work note ls 1
work note edit 3 --at 1330
work status
work end 1402
work log --today
work log --since 14d
work log --since 14d -p someproject
work log --date 2026-05-25
work week -p someproject
work edit 1 --start 0806 --end 1430
work db path
```

Data is stored in SQLite at `~/.local/share/work-cli/work.sqlite` by default.
Set `WORK_DB` to use another path.

## Projects

```bash
work project add someproject
work project set someproject --weekly 20h --workdays mon,tue,thu,fri
work project balance someproject --set 80h
work project list
work start 800
```

When multiple active projects exist and no `-p/--project` is given,
`work start` opens an interactive project picker.

## Weekly targets and overtime

Projects can have weekly target hours and configured workdays:

```bash
work project set someproject --weekly 20h --workdays mon,tue,thu,fri
```

This means `someproject` should get 20 hours per week, spread across Monday,
Tuesday, Thursday, and Friday. Wednesday is not planned for that project.

Set the current overtime balance with:

```bash
work project balance someproject --set 80h
```

The optional `--date YYYY-MM-DD` flag records the balance adjustment date:

```bash
work project balance someproject --set 80h --date 2026-06-03
```

Show the weekly projection with:

```bash
work week -p someproject
```

If only one active project exists, the project name can be omitted for
`work week`, `work project set`, and `work project balance`. When multiple
projects exist, the CLI opens the project picker.

`work week` shows:

- `week`: time already worked this week against the weekly target.
- `left`: remaining time to hit the weekly target.
- `schedule`: all configured workdays for the project.
- `remaining`: configured workdays still left in the selected week.
- `per day`: remaining time split across the remaining workdays.
- `deadline`: last remaining configured workday in the selected week.
- `balance`: current overtime balance, including completed week deltas since
  the last manual balance set.
- `projected`: expected balance at the selected week end if no more time is
  logged in that week.

For example, with a `+78h` balance, `11h 16m` worked, and a `20h` weekly
target, the projected balance is:

```text
+78h + 11h 16m - 20h = +69h 16m
```

`work status` also shows a `weekly` section for projects with configured weekly
targets. It includes today's project target, how long is left today, the time to
work until when a target remains, the `over today` amount when the target is
already exceeded or the selected day is not a configured workday, and the current
overtime balance with completed weeks already applied.
