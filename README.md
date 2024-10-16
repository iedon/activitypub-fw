# ActivityPub-FW (Under Construction)

A middleware inspired by [great-ebichiri-wall](https://github.com/shrimpia/great-ebichiri-wall) to filter incoming inbox messages. However, **in local**.

We do not need to take care about running out of free daily requests when using an online worker, especially in huge spam waves.

![Architecture](arch.png)

# Usage
Use as a middleware in front of an activity pub api server and behind your front proxy server(usually nginx).

```bash
Usage: ./activitypub-fw [-c config_file]
  -c string
        Path to the JSON configuration file (default "config.json")
  -h    Print this message
```

# Configuration
```json
{
    "server": {
        "address": "[::]", // Where we listen. Configure your front proxy server(usually nginx) to contact this. Only works if protocol is "tcp". IPv6 address should be in "[]"
        "path": "/var/run/activitypub-fw.sock", // Where we listen. Configure your front proxy server(usually nginx) to contact this. Only works if protocol is "unix"
        "port": 8080, // Only works if protocol is "tcp"
        "protocol": "tcp", // "tcp" or "unix"
        "readTimeout": 5, // Second(s)
        "writeTimeout": 10, // Second(s)
        "idleTimeout": 120 // Second(s)
    },
    "proxy": {
        "protocol": "tcp", // "tcp" or "unix"
        "url": "http://172.23.91.135", // Only works if protocol is "tcp"
        "unixPath": "/var/run/mastodon-website.sock", // Points to backend server. eg. mastodon website(not streaming)'s domain socket file. Only works if protocol is "unix"
        "forceAttemptHttp2": true,
        "maxConnsPerHost": 256,
        "maxIdleConns": 100,
        "maxIdleConnsPerHost": 5,
        "idleConnTimeout": 120, // Second(s)
        "tlsHandshakeTimeout": 10, // Second(s)
        "expectContinueTimeout": 1, // Second(s)
        "keepAlive": 30, // Second(s)
        "Timeout": 30, // Second(s)
        "writeBufferSize": 4096, // Bytes
        "readBufferSize": 4096 // Bytes
    }
}
```

# Set-up
Assume you are running a mastodon instance behind a proxy server(nginx).

Here is a sample nginx configuration section.

```
# Comment original upstream "website"
#upstream backend {
# server unix:/var/run/mastodon-website.sock fail_timeout=0;
#}
# Change to use this project(acts as a middleware, which will contact ```/var/run/mastodon-website.sock``` above instead)
upstream backend {
 server unix:/var/run/activitypub-fw.sock fail_timeout=0;
}
```

# Functions
- ✅ Basic filtering by ```At(@)``` and ```Mentions(cc)``` in an ActivityPub message
- Make an easy-to-use UI
- Blacklist support for content filtering
- Real time analyzation
- Further integrations: AI..., anti-spam interfaces
- ...