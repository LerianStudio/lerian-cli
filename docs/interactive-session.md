# The interactive session

`lerian` with nothing after it opens a session: it offers the commands, runs the
one you pick, and asks again when that command is done. `q` closes it.

```
  ██╗     ███████╗██████╗ ██╗ █████╗ ███╗   ██╗        ██████╗██╗     ██╗
  ██║     ██╔════╝██╔══██╗██║██╔══██╗████╗  ██║       ██╔════╝██║     ██║
  ██║     █████╗  ██████╔╝██║███████║██╔██╗ ██║ █████╗██║     ██║     ██║
  ██║     ██╔══╝  ██╔══██╗██║██╔══██║██║╚██╗██║ ╚════╝██║     ██║     ██║
  ███████╗███████╗██║  ██║██║██║  ██║██║ ╚████║       ╚██████╗███████╗██║
  ╚══════╝╚══════╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝        ╚═════╝╚══════╝╚═╝

  ─────────────────────────────────────────────────────────────  v1.7.0

  What do you want to do?
  ↑↓ move · enter choose · r back · q cancel
  ❯ auth     Authentication commands
    infra    Deploy the AWS stacks of lerian-terraform-foundation
    version  Print version information
```

Choosing a command that only groups others — `auth` — offers its subcommands
rather than printing a help page:

```
  Which auth command?
  ❯ login   Configure authentication credentials
    logout  Remove authentication credentials
```

The menu is a shorter list than the command set. `midaz` is reached with ledger
ids, regions and sizes a menu has no way to ask for, so picking it from a list
would land you on a help page rather than on anything you chose to do — it stays
a command (`lerian midaz ledger list` is unaffected) and stays out of the menu.

`r` goes back one question; `q` leaves. The questions come in a sequence, and a
wrong turn on the first one used to cost the whole run — the only way to correct
it was ctrl-c, which throws away the answers that were right along with the one
that was not. On the first question, and on the main menu, there is nothing
before: `r` there backs out of the run, and out of nothing, respectively.

The session exists because these commands come in sequences — check the machine,
then init; init fails, read what it says, run it again — and each of those used
to cost a fresh start. A command that fails does not close it either. The exit
code is the last command's, so `lerian && something` still means what it says.

The wordmark lights up a line at a time and the rule draws itself across, which
takes about a fifth of a second and happens once per session. Below the width the
wordmark needs it gives way to a single line — a wrapped wordmark is six broken
lines, and the terminal decides where it wraps.

Only on a terminal. Piped, redirected or in CI, `lerian` prints its help exactly
as before: a prompt there waits for an answer that is never coming. The banner
follows the same rule, and `LERIAN_NO_BANNER=1` turns it off for anyone who has
seen it enough times. `NO_COLOR` and `TERM=dumb` are honored throughout — both
skip the animation and print the banner whole.
