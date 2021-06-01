# IDO Cleanup Tool

In larger installations of Icinga 2, [IDO DB cleanup] can become a challenge. Those cleanup queries are scheduled in
between any other INSERT or UPDATE query.

This can cause 2 problems:
- Regular updates are deferred when cleanup is running
- Not all tables are indexed correctly, so cleanup can take longer

This tool is build to realize IDO cleanup outside of Icinga 2, so you should disable any `cleanup` inside
`/etc/icinga2/features-available/ido*.conf` and setup this tool as service.

Currently, only MySQL is supported.

[IDO DB cleanup]: https://icinga.com/docs/icinga-2/latest/doc/14-features/#db-ido-cleanup

## Install

You can download the built binary from [releases](https://github.com/NETWAYS/ido-cleanup/releases).

## Usage

```
$ export DB_DSN='icinga:icinga@tcp(database:3306)/icinga'
$ ./ido-cleanup
```

```
Usage of ido-cleanup:
      --db string                         DB Connecting string (env:DB_DSN) (default "icinga:icinga@/icinga2")
      --instance string                   IDO instance name (default "default")
      --limit int                         Limit deleting rows in one query (default 10000)
      --interval duration                 Cleanup every X seconds (default 1m0s)
      --fast-interval duration            Cleanup every X seconds - when more then 2x limit rows to delete (default 10s)
      --once                              Just run once
      --noop                              Just check - don't purge
      --debug                             Enable debug logging
      --acknowledgements uint             How long to keep entries of acknowledgements in days
      --commenthistory uint               How long to keep entries of commenthistory in days (default 365)
      --contactnotifications uint         How long to keep entries of contactnotifications in days (default 365)
      --contactnotificationmethods uint   How long to keep entries of contactnotificationmethods in days
      --downtimehistory uint              How long to keep entries of downtimehistory in days (default 365)
      --eventhandlers uint                How long to keep entries of eventhandlers in days (default 365)
      --externalcommands uint             How long to keep entries of externalcommands in days
      --flappinghistory uint              How long to keep entries of flappinghistory in days
      --hostchecks uint                   How long to keep entries of hostchecks in days
      --logentries uint                   How long to keep entries of logentries in days (default 365)
      --notifications uint                How long to keep entries of notifications in days (default 365)
      --processevents uint                How long to keep entries of processevents in days
      --statehistory uint                 How long to keep entries of statehistory in days (default 365)
      --servicechecks uint                How long to keep entries of servicechecks in days
      --systemcommands uint               How long to keep entries of systemcommands in days
```

## Example

```
$ ido-cleanup --once
INFO[0000] starting ido-cleanup                         
INFO[0000] deleted rows  oldest="2019-03-01 11:26:44 +0000 UTC" rows=10000 table=commenthistory took=72.647379ms
INFO[0000] deleted rows  oldest="2019-01-21 19:35:14 +0000 UTC" rows=6129 table=contactnotifications took=76.247963ms
INFO[0000] deleted rows  oldest="2019-02-06 09:42:08 +0000 UTC" rows=209 table=downtimehistory took=3.992535ms
INFO[0000] deleted rows  oldest="2019-04-23 07:29:36 +0000 UTC" rows=60 table=eventhandlers took=6.685573ms
INFO[0000] deleted rows  oldest="2019-01-21 19:35:14 +0000 UTC" rows=6212 table=notifications took=84.653149ms
INFO[0000] deleted rows  oldest="2019-01-20 10:16:30 +0000 UTC" rows=10000 table=statehistory took=135.619702ms
INFO[0000] stopping after one cleanup       
```

## Missing indices

Not all tables are indexed in a way to support fast deletions. 

Please add those indexes to ensure speedy queries.

```mysql
CREATE INDEX idx_notifications_cleanup ON icinga_notifications (`instance_id`,`start_time`);
CREATE INDEX idx_contactnotifications_cleanup on icinga_contactnotifications (instance_id, start_time);
```

See [Icinga/icinga2#7753](https://github.com/Icinga/icinga2/issues/7753).

## License

Copyright (C) 2021 [NETWAYS GmbH](mailto:info@netways.de)

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
