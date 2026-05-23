# Grafana

### Login As Postgres User

`sudo -u postgres psql -d postgres`

### Bootstrap Grafana Postgres User

```sql
CREATE USER grafana WITH PASSWORD '???';
GRANT ALL ON SCHEMA public TO grafana;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO grafana;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO grafana;
```
