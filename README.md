# statsd-router
Rule based statsd router written in Go.

## Usage
```
$ ./statsd-router -h
Usage of ./statsd-router:
  -api-port uint
    	Port for API to use (default 48126)
  -bind-address string
    	Address to bind (default "0.0.0.0")
  -check-interval int
    	Interval of checking for backend health (default 180)
  -config string
    	Configuration file path (default "statsd-router.json")
  -debug
    	Enable debug mode
  -master-statsd-host string
    	Host that will receive all metrics. Format is host:port:mgmt_port (default "localhost:8125:8126")
  -port uint
    	Port to use (default 48125)
```

## Config file format
```
$ cat statsd-router.json
{
  "rules": {
    ".*apps\\.admin\\.demo\\..*": [
      {
        "host": "localhost",
        "port": 18125,
        "mgmt_port": 18126
      },
      {
        "host": "localhost",
        "port": 28125,
        "mgmt_port": 28126
      }
    ],
    ".*apps\\.admin\\.test\\..*": [
      {
        "host": "localhost",
        "port": 28125,
        "mgmt_port": 28126
      }
    ]
  }
}
```

## API

### List all rules

```
$ curl http://localhost:48126/rules
{
  "rules": {
    ".*apps\\.admin\\.demo\\..*": [
      {
        "host": "localhost",
        "port": 18125,
        "mgmt_port": 18126
      },
      {
        "host": "localhost",
        "port": 28125,
        "mgmt_port": 28126
      }
    ],
    ".*apps\\.admin\\.test\\..*": [
      {
        "host": "localhost",
        "port": 28125,
        "mgmt_port": 28126
      }
    ]
  }
}
```

### Add new rule

```
$ curl -X POST -H 'Content-Type: application/json' http://localhost:48126/rules --data '{"rules": {".*apps\\.admin\\.demo\\..*": [{"host": "localhost","port": 8080,"mgmt_port": 8181},{"host": "localhost","port": 9090,"mgmt_port": 9191}]}}'
{"message":"The config was successfully updated."}
```
