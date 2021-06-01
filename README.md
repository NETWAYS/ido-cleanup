# IDO Cleanup Tool

## Missing indices

Not all tables are indexed in a way to support fast deletions. 

Please add those indexes to ensure speedy queries.

```mysql
CREATE INDEX idx_notifications_cleanup ON icinga_notifications (`instance_id`,`start_time`);
```

See [Icinga/icinga2#7753](https://github.com/Icinga/icinga2/issues/7753).
