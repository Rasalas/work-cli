# work-cli

A small local time-tracking CLI.

## Usage

```bash
work start 800 -p someproject
work do "parser debuggen"
work doing "sqlite migration pruefen"
work done "migration laeuft"
work status
work end
work log --today
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
