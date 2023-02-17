# Procfile Static Webserver Sample Application

## Building

```bash
pack build applications/procfile
```

## Running

```bash
docker run --tty --publish 8080:8080 applications/procfile
```

## Viewing

```bash
curl -s http://localhost:8080/hello-world.txt
```
