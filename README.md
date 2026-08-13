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
work end --at-target
work end --use-overtime
work end 1402 --use-overtime
work end 1402 --use-overtime=1h
work log --today
work log --since 14d
work log --since 14d -p someproject
work log --date 2026-05-25
work week -p someproject
work absence add -p someproject --from 2026-07-17 --to 2026-08-03 --type vacation
work absence list -p someproject
work export -p someproject --month 2026-08
work export -p someproject --month 2026-08 --show-overtime
work edit 1 --start 0806 --end 1430
work db path
```

Data is stored in SQLite at `~/.local/share/work-cli/work.sqlite` by default.
Set `WORK_DB` to use another path.

## Projects

```bash
work project add someproject
work project set someproject --weekly 20h --workdays mon,tue,thu,fri --report-start 0800 --report-end 1300
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

End a session now and use enough overtime to reach today's planned target:

```bash
work end --use-overtime
```

To use a fixed end time instead:

```bash
work end 1400 --use-overtime
```

Use an explicit amount instead with the `=<duration>` form:

```bash
work end 1400 --use-overtime=1h
```

The session keeps its actual end time. Overtime use is stored separately,
reduces the balance immediately, and counts toward the weekly target without
being deducted a second time when the week completes.

If a forgotten pause leaves a session running beyond today's planned target,
end it exactly when the target was reached:

```bash
work end --at-target
```

Earlier sessions and overtime already used on the same day are included. The
command refuses to end the session in the future when the target has not been
reached yet. Use `--use-overtime` instead to end now and cover the remainder.
Because that keeps the actual end time while `--at-target` moves it backward,
the flags cannot be combined. An explicit reference time can be supplied for a
correction:

```bash
work end 1530 --at-target
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
targets. It includes today's planned project target, calculated by spreading the
weekly target across the configured workdays, how long is left for that planned
target, the time to work until when a target remains, the `over today` amount
when the target is already exceeded or the selected day is not a configured
workday, and the current overtime balance with completed weeks already applied.

## Absences

Record vacation or another planned absence as an inclusive date range:

```bash
work absence add -p someproject \
  --from 2026-07-17 \
  --to 2026-08-03 \
  --type vacation
```

Only configured project workdays reduce the target. Weekends and other
non-workdays inside the range are ignored. A full absent workweek therefore has
a zero-hour target and no longer reduces the overtime balance. Partial weeks
reduce the target by the planned daily share for every absent workday.

List recorded absences with:

```bash
work absence list -p someproject
```

Overlapping absence ranges for the same project are rejected.

## Export

Configure the employer reporting window for a project. Reporting settings can
be added independently of the weekly schedule:

```bash
work project set someproject --report-start 0800 --report-end 1300
```

Export the current month as CSV, or select a month or date explicitly:

```bash
work export -p someproject
work export -p someproject --month 2026-08
work export -p someproject --date 2026-08-04
```

The default export emits one compact employer-facing row per recorded day:

```csv
date,start,end,type,duration
2026-08-04,08:00,13:00,work,05:00
```

Use `--show-overtime` to split the same reported interval into work and
overtime-use rows while keeping the CSV columns and total interval unchanged:

```bash
work export -p someproject --month 2026-08 --show-overtime
```

```csv
date,start,end,type,duration
2026-08-04,08:00,11:30,work,03:30
2026-08-04,11:30,13:00,overtime_use,01:30
```

Redirect stdout to save the export:

```bash
work export -p someproject --month 2026-08 > work-2026-08.csv
```
