# ActivityPub-FW (Under Construction)

A middleware inspired by great-pari-wall to filter incoming inbox messages, locally.

# Usage
Use as a middleware ahead of a activity pub api server and behind your nginx or front proxy.

# Nginx Configuration <TODO>
here is a sample nginx configuration section.
```
location / {
    proxy_pass http:///unix:/var/run/activitypub-fw.sock;
}
```