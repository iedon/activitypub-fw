module github.com/iedon/activitypub-fw

go 1.23.2

replace github.com/iedon/activitypub-fw/proxy => ./proxy

replace github.com/iedon/activitypub-fw/config => ./config

require (
	github.com/fsnotify/fsnotify v1.7.0
	golang.org/x/sys v0.4.0 // indirect
)
