# Cluster Service Version generation

Since we got bored with manual work on CSV generation, I'd like to infroduce you the automated way
how we can make our lives easier.
You need to install [ytt](https://carvel.dev/ytt/docs/latest/install/) and then run

```bash
cd ./deploy/csv
ytt -f ytt \
    --data-value-file operator_manifest='../../deploy/operator.yaml' \
    --data-value-file cr_manifest='../../deploy/cr.yaml' \
    --data-value-file restore_manifest='../../deploy/backup/restore.yaml' \
    --data-value-file backup_manifest='../../deploy/backup/backup.yaml' \
    --data-value-file secrets_manifest='../../deploy/secrets.yaml' \ --data-value-file rn_txt='./ytt/ReleaseNotes.md' \
    --data-value rbac_manifest="$(yq r -d0 ../../deploy/rbac.yaml)"
```
