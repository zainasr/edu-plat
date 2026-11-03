# 01. Database concepts: transactions, standby, replication, PITR and backups

Each concept starts with a plain-language definition, then a concrete example from this product.

## 1. Transaction (ACID)

A transaction groups several statements so they **all** happen or **none** do.

```sql
begin;
  update accounts set balance = balance - 1200000 where id = 'cash';
  update accounts set balance = balance + 1200000 where id = 'receivable';
commit;      -- or: rollback;
```
- **Atomic:** both updates or neither.
- **Consistent:** rules (constraints, debit = credit) still hold after commit.
- **Isolated:** concurrent transactions do not see each other's half-done work.
- **Durable:** once `commit` returns, the data survives a crash (written to the write-ahead log, WAL, first).

Node equivalent: `await client.query('BEGIN')` … `COMMIT` / `ROLLBACK` with `pg`, or Prisma's `$transaction`. In Go with `pgx`: `tx, err := pool.Begin(ctx)`, then `tx.Commit(ctx)` or `tx.Rollback(ctx)`, with `defer tx.Rollback(ctx)` as a safety net.

**Why it matters here:** a payment must never leave the receipt written but the journal missing.

## 2. Write-ahead log (WAL): the foundation of everything below

Postgres first writes every change to a log file, the WAL, and only then to the data files. The WAL is a complete, ordered history of changes. Standby replicas and point-in-time recovery both use it.

## 3. Standby (replica) and replication

A **standby** is a second Postgres server that continuously receives the WAL from the primary and replays it. It holds almost the same data.

```
Primary (Bangalore, node A)  --- WAL stream --->  Standby (node B)
   accepts reads and writes                          read-only (or waiting to be promoted)
```

| Mode | Behaviour | Trade-off |
|---|---|---|
| **Asynchronous** | Primary commits without waiting for the standby | Fast; a crash can lose the last few transactions (the **RPO**, recovery point objective) |
| **Synchronous** | Primary waits until the standby confirms | No loss of committed data; slower writes; if the standby is down, writes can stall |

**Failover:** if the primary dies, you **promote** the standby (it becomes the new primary) and point the app at it. The **RTO** (recovery time objective) is how long that takes.

**What a standby does NOT protect against:** a bad `DELETE` or `DROP TABLE`. The standby replays that too, within milliseconds. This is the single most important thing to understand about standbys.

### Mock timeline
```
10:00:00  primary running, standby 1 second behind
10:05:00  a developer runs `delete from challans` on the wrong tenant
10:05:01  standby also deletes it
10:05:02  both copies are now wrong. A standby cannot help here.
```
That is where PITR comes in.

## 4. Point-in-time recovery (PITR)

PITR restores the database to **any moment** you choose, for example "10:04:59".

It needs two things:
1. A **base backup**: a full copy of the data directory taken at some time (for example every night).
2. **Continuous WAL archiving**: every WAL segment is copied off the server (to DigitalOcean Spaces, object storage) as it fills.

To restore, you take the base backup and replay the archived WAL **up to the target time**, then stop.

Sketch of a recovery configuration (Postgres 12+ uses a `recovery.signal` file and these settings):
```ini
restore_command = 'aws s3 cp s3://our-backups/wal/%f %p'   # fetch archived WAL segments
recovery_target_time = '2026-11-01 10:04:59+05'            # replay up to this moment
recovery_target_action = 'promote'                         # then open as a normal database
```
(The exact tool for copying WAL depends on the backup software chosen later, for example pgBackRest or WAL-G. The concept stays the same.)

**Mock recovery:**
```
Monday 02:00  base backup taken
Monday 02:00-10:04:59  WAL archived continuously
10:05:00  bad delete happens
Action    restore the base backup to a new server, replay WAL to 10:04:59, check the data,
          then copy the recovered rows back or switch the application
```
Data written after 10:04:59 is lost by design. That is the trade-off: you choose the moment.

## 5. Standby vs PITR (the table to remember)

| | Standby (replication) | PITR (backup + WAL archive) |
|---|---|---|
| Protects against | Server or disk failure | Human error, bad deploy, corrupted data |
| Recovery speed | Seconds to minutes (promotion) | Minutes to hours (restore and replay) |
| Point in time | Now (latest) | Any moment in the retention window |
| Typical RPO | Near zero (async: seconds) | Minutes (last archived WAL) |
| Cost | A second server | Storage for base backups and WAL |

**You need both.** Standby gives availability; PITR gives recoverability.

## 6. Backups: the rule and the test

- **3-2-1 rule:** three copies, on two different media, one copy off-site. We use: the live primary, a standby, and off-site object storage in another region (ADR-0006).
- **A backup you have not restored is not a backup.** Quarterly restore drills are in the NFR spec. Write the procedure in `docs/runbooks/` when built.

## 7. Row-level security (RLS) in one minute

RLS makes Postgres filter rows per user or per setting, so a query without the right `WHERE` still returns only one school's rows.

```sql
create policy tenant_isolation on students
  using (tenant_id = current_setting('app.tenant_id')::uuid);
```
Our app sets `app.tenant_id` at the start of each transaction. Details and the reasoning are in ADR-0002.

## 8. Indexes in one minute (for later)

An index is a sorted lookup structure, like the index at the back of a book. Without one, Postgres reads the whole table to find a row. Every tenant table has an index that starts with `tenant_id` (rules in `.claude/rules/sql-migrations.md`). We will look at `EXPLAIN` plans when the first list screen is slow, not before.

## 9. Migrations (the schema history)

Schema changes are versioned SQL files, applied in order, and never edited after they ship. Tool: `goose` (or `atlas`, still being decided in design). Expand, then migrate data, then contract, across releases, so old and new app versions both work during a deploy.

## Read next
- Postgres docs, "Transactions" and "High Availability, Load Balancing, and Replication": https://www.postgresql.org/docs/current/
- pgBackRest user guide (a common PITR tool): https://pgbackrest.org/user-guide.html
- The Art of PostgreSQL (for SQL depth): https://theartofpostgresql.com/
