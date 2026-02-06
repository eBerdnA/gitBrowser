# GitBrowser

GitBrowser is a highly opinionated project.
It is built for a very specific personal workflow, and I am happy if others can benefit from it too.  
**It should not be exposed to the public internet.**

## run

```bash
cd /gitBrowser
CGO_ENABLED=0 go run main.go
```

## configure repositories

GitBrowser reads repository config from `repos.json` by default.
You can set a different path with `GITBROWSER_CONFIG`.

Example `repos.json`:

```json
{
  "repos": [
    { "name": "gitBrowser", "path": "." },
    { "name": "my-other-repo", "path": "/absolute/path/to/repo" }
  ]
}
```

## docker compose example

A `docker-compose.yml` example is included in this repo.

1. Update the repository volume mounts in `docker-compose.yml` to point to real host paths.
2. Make sure `repos.json` uses the matching container paths (for example `/repos/app1` and `/repos/app2`).
3. Start the app:

```bash
docker compose up
```

Example `repos.json` for the included compose file:

```json
{
  "repos": [
    { "name": "app1", "path": "/repos/app1" },
    { "name": "app2", "path": "/repos/app2" }
  ]
}
```
