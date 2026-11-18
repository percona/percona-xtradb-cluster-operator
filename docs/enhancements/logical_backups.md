# Logical backups and restores

## Summary

The operator can take a logical (SQL-level) backup of a PXC cluster with
[mydumper](https://github.com/mydumper/mydumper) and restore it with myloader. It uses the same
two custom resources as physical backups.

A backup is requested with `type: logical`:

```yaml
apiVersion: pxc.percona.com/v1
kind: PerconaXtraDBClusterBackup
metadata:
  name: logical1
spec:
  pxcCluster: cluster1
  storageName: s3-us-west
  type: logical
```

A job dumps one PXC node and uploads the dump to the S3 storage, one object per mydumper file.

A restore points at the backup, as it does for a physical one:

```yaml
apiVersion: pxc.percona.com/v1
kind: PerconaXtraDBClusterRestore
metadata:
  name: restore1
spec:
  pxcCluster: cluster1
  backupName: logical1
```

The cluster stays up. A job downloads the dump to a temporary volume and loads it into the
running cluster through the first PXC pod. Tables that are in the dump are dropped and recreated;
everything else is left alone.

## Goals

- Take a logical backup of a cluster to S3 storage, on demand and on a schedule.
- Restore a logical backup into a running cluster, from a backup object or from a `backupSource`.
- Reuse the existing backup and restore custom resources and their workflow: storages, deadlines,
  the per-cluster backup lock, finalizers and status reporting.
- Let users pass any mydumper or myloader option without a typed API field for each.
- Leave physical backups and restores unchanged. A backup or restore that does not ask for
  `logical` builds the same jobs and destinations as before.

## Non-goals

- Azure and PVC storage for logical backups.
- Point-in-time recovery on top of a logical backup.
- Typed fields for schema selection, thread counts or other tool options.
- Backing up and restoring users and grants. The `mysql` and `sys` schemas are not dumped.
- A rollback of a failed restore.
- Shipping mydumper in an official backup image. The image set in `spec.backup.image` must
  contain `mydumper`, `myloader` and the `aws` CLI.

## API changes

Every new field is optional.

### `PerconaXtraDBClusterBackup`

| Field | Type | Description |
|---|---|---|
| `spec.type` | `full` \| `logical` | The kind of backup. `full` is a physical backup taken with xtrabackup. Defaults to `full`. |
| `status.type` | `full` \| `logical` | The kind of backup that was taken. Empty on backups taken before this field existed, and read as `full`. |
| `status.size` | integer | Total size in bytes of the backup's objects in the storage. Set for logical backups only. `0` when it could not be determined. |

The backup's printed columns gain `Type`.

### `PerconaXtraDBCluster`

| Field | Type | Description |
|---|---|---|
| `spec.backup.schedule[].type` | `full` \| `logical` | The kind of backup the schedule creates. Defaults to `full`. Changing it recreates the schedule's cron entry. |
| `containerOptions.args.mydumper` | list of strings | Extra arguments for mydumper. Used by logical backups only. |
| `containerOptions.args.myloader` | list of strings | Extra arguments for myloader. Used by restores of logical backups only. |

`containerOptions` exists on a storage, on a backup and on a restore, as before. A backup's own
options take precedence over its storage's.

### `PerconaXtraDBClusterRestore`

| Field | Type | Description |
|---|---|---|
| `spec.job.volume.size` | quantity | Size of the temporary volume the dump is downloaded to. Defaults to the backup size plus 10%, with a minimum of 1Gi. |
| `spec.job.volume.storageClassName` | string | Storage class of that volume. Defaults to the cluster's default storage class. |
| `spec.backupSource.type` | `full` \| `logical` | The kind of backup at the destination. `backupSource` reuses the backup status type, so this is `status.type` above. |
| `status.backupType` | `full` \| `logical` | The kind of backup being restored, resolved when the restore starts. |

`spec.job` holds settings of the restore job, so that more can be added later. `spec.job.volume`
is rejected on a restore of a physical backup.

Example with the optional fields:

```yaml
apiVersion: pxc.percona.com/v1
kind: PerconaXtraDBClusterRestore
metadata:
  name: restore1
spec:
  pxcCluster: cluster1
  backupName: logical1
  containerOptions:
    args:
      myloader: ["--threads=8"]
  job:
    volume:
      size: 50Gi
      storageClassName: fast
```

### Validation

A logical backup fails, with the reason in `status.error`, when:

- the storage is not of type `s3`;
- the cluster's `crVersion` is below `1.21.0`.

A logical restore fails, with the reason in `status.comments`, when:

- `spec.pitr` is set;
- the cluster's `crVersion` is below `1.21.0`;
- the backup's destination is not on S3;
- the cluster is not `ready`;
- the bucket cannot be listed, or the destination has no `metadata` object;
- a secret the job refers to does not exist.

## Design decisions

### One `type` field

`type` takes `full` or `logical`. A separate field for the backup method was considered and
dropped: a backup is either physical or logical, so one field leaves no invalid combination to
reject. `full` keeps its name for consistency with `percona-server-mysql-operator`, which uses
`type: full | incremental`. Incremental backups are planned for this operator with the same field,
giving `full | incremental | logical`.

### Tool options are free-form

mydumper and myloader options go in `containerOptions.args.mydumper` and
`containerOptions.args.myloader`, next to the existing `xtrabackup`, `xbcloud` and `xbstream`
lists. Thread counts, schema filters and compression are set this way.

The scripts add a few defaults. A default is left out when the user's arguments set the same
option; the scripts check for this themselves, because mydumper does not treat a repeated option
consistently.

| Tool | Default | Overridable |
|---|---|---|
| mydumper | `--sync-thread-lock-mode=GTID` | yes |
| mydumper | `--regex='^(?!(mysql\|sys)(\.\|$))'` | yes, with `--regex` or `-x` |
| mydumper | `--chunk-filesize=64`, `--exec-threads=4` | yes: the user's value is passed after the default (not verified for these two options) |
| mydumper | `--triggers --routines --events` | no |
| mydumper | `--exec`, `--outputdir`, `--host`, `--user` | no |
| myloader | `--drop-table` | yes, with `--drop-table` or `-o` |
| myloader | `--directory`, `--host`, `--user` | no |

### The dump is uploaded file by file

mydumper runs with `--exec`, which calls an upload script for each file as soon as it is
complete. The bucket ends up with a normal mydumper directory:

```
s3://<bucket>/<prefix>/<cluster>-<creation time>-logical/metadata
s3://<bucket>/<prefix>/<cluster>-<creation time>-logical/shop-schema-create.sql
s3://<bucket>/<prefix>/<cluster>-<creation time>-logical/shop.orders.00000.sql
```

mydumper's stream mode, which produces a single object, was considered and dropped in favour of a
layout that can be browsed and restored by hand with standard tools.

`--chunk-filesize` keeps files small, so the backup job needs little scratch space and uses an
`emptyDir`. In testing, a 280 MB dump peaked at 174 MB of scratch.

Three behaviours of mydumper shape the script:

- It deletes each file after the upload script returns, even when the upload failed, and still
  exits 0. The upload script tries three times and records a failure in a marker file.
- It never passes its `metadata` file to `--exec`. The backup script uploads that file itself,
  last.
- A dump that fails partway can still leave a `metadata` file behind.

The backup succeeds only when mydumper exited 0, no upload failed, and `metadata` is in the
bucket.

### mydumper runs in GTID lock mode

mydumper's default lock mode takes `LOCK INSTANCE FOR BACKUP` and then
`FLUSH TABLES WITH READ LOCK`. On PXC 8.4 this is unsafe:

- After that sequence the node stays in `Donor/Desynced` until it is restarted. The backup
  scripts only choose `Synced` nodes as a source, so each such dump removes a candidate, for
  physical backups too.
- With writes and a long-running query on the same table of the dumped node, the read lock never
  returned and the node hung until it was killed. In one run, writes to the whole cluster stalled
  for about 14.5 minutes.

`GTID` mode takes no locks. In testing under write load it completed every run and produced
consistent dumps. A user can choose another mode through the `mydumper` arguments.

### The dump source is chosen as for physical backups

The job connects directly to one PXC pod, not through the proxy. It picks a synced node of the
primary component and avoids the first pod when the cluster has more than one, using the same
function as the physical backup script.

### The restore runs against a live cluster

A physical restore stops the cluster and replaces a data directory. A logical restore loads SQL,
so the cluster has to be up, and the restore never changes the cluster spec. Its states are
`Starting` → `Restoring` → `Succeeded` or `Failed`.

The job writes through the first PXC pod, which is where the proxy sends writes by default, so
the load does not conflict with application writes arriving on another node.

### The dump is downloaded to a temporary volume

myloader reads a directory, so the whole dump is downloaded first. A physical restore unpacks
onto the first pod's data volume, which is free because the cluster is stopped. During a logical
restore that volume is in use, so the job gets its own.

The volume is a generic ephemeral volume. Kubernetes creates its claim with the job's pod and
deletes it with the pod, so the operator has no cleanup to do and needs no new permissions.

Its size is the backup's `status.size` plus 10%, with a 1Gi minimum. When no size was recorded,
the operator sums the objects in the bucket. `spec.job.volume.size` overrides both.

### The operator records the backup size

After a logical backup's job succeeds, the operator lists the backup's objects and writes the
total to `status.size`. If the listing fails, the backup still succeeds and the size stays `0`.

### The backup type is resolved once per restore

The restore controller decides in the first state whether the restore is logical and records the
answer in `status.backupType`. Later states read it and never decide again, because only the
physical path stops the cluster and a restore must not change paths halfway.

- A backup object states its type. One with no type was taken before types existed and is
  physical.
- A `backupSource` that states its type is trusted.
- A `backupSource` on S3 with no type is checked against the bucket: a `metadata` object under
  the destination means a logical backup. Without this check, a restore pointed at a logical dump
  would take the physical path, stop the cluster, and then fail. If the bucket cannot be read,
  the restore waits and retries.

### Views get a primary key on restore

For each view, mydumper writes a placeholder table so that views can be created in any order. The
placeholder has no primary key, and PXC with `pxc_strict_mode=ENFORCING` refuses to create it, by
which point myloader has already dropped the existing tables. The restore script adds a primary
key column to each placeholder before loading. myloader drops the placeholders when it creates
the views.

### Logical backups stay out of PITR bookkeeping

A logical backup cannot be the base of a point-in-time recovery. It is skipped when the operator
picks the latest backup for PITR status, and it does not reset the binlog collector's gap
tracking. Otherwise a logical backup would hide a binlog gap that affects the last physical
backup.

### The password is passed through the environment

mydumper and myloader get the `xtrabackup` user's password in `MYSQL_PWD`. A defaults file was
tried first and failed for a generated password containing `#`, which both tools misread when it
was not quoted.

### Physical and logical restores are separate packages

`pkg/controller/pxcrestore` resolves the backup type and hands the restore to a controller that
implements `Restore(ctx)`:

| Package | Role |
|---|---|
| `pxcrestore/physical` | The existing state machine and storage restorers. |
| `pxcrestore/logical` | The logical state machine, its validation and volume sizing. |
| `pxcrestore/common` | The dependencies both are built from, and shared helpers. |

The backup side has no such split. The backup controller branches on the type only to validate,
to pick the job spec and destination name, and to record the size.

### `crVersion` gate

Logical backups and restores require `crVersion` `1.21.0`. The jobs get their scripts from the
init image, and for a cluster on an older `crVersion` the operator uses the init image of that
version, which does not contain them.

## Limitations

- **Only S3 storage is supported.**
- **A restore overwrites live data.** Tables in the dump are dropped and recreated while
  applications may be connected.
- **A failed restore leaves tables partly loaded.** Running the restore again is safe, since it
  drops and recreates the same tables.
- **A restore pauses other writes on the cluster.** In testing, a 22-second restore paused
  unrelated writes for up to 5.5 seconds, and for 1.7 seconds with `--optimize-keys=SKIP`. The
  pause is expected to grow with table size.
- **A restore needs a volume as large as the dump.**
- **A restore needs the operator to be able to list the bucket.**
- **A schema change during a dump fails the backup.** GTID mode takes no DDL lock.
- **GTID mode is described by mydumper as best effort.** Its threads reached the same position
  in every test run. What it does when they cannot was not observed.
- **Users and grants are not restored.**
- **A schema filter can leave out `CREATE DATABASE`.** A pattern such as `^shop\.` dumps the
  tables of `shop` but not the statement that creates the database, so that dump restores only
  where `shop` already exists.
- **A restore can report success a few seconds before every node has applied it.**
- **A backup taken during a logical restore captures a partly restored cluster.** No locking
  prevents it.
- **No automated test covers a backup failing because an upload failed.** That path was checked
  by hand.
- **Tested with mydumper 1.0.19 on PXC 8.4 only.**
