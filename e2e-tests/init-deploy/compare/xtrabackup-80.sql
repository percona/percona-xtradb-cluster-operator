GRANT RELOAD, PROCESS, LOCK TABLES, REPLICATION CLIENT ON *.* TO `xtrabackup`@`%`
GRANT BACKUP_ADMIN,RESOURCE_GROUP_USER,SERVICE_CONNECTION_ADMIN ON *.* TO `xtrabackup`@`%`
GRANT SELECT ON `performance_schema`.`keyring_component_status` TO `xtrabackup`@`%`
GRANT SELECT ON `performance_schema`.`log_status` TO `xtrabackup`@`%`