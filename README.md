# TinyC

> A simple container implementation in Go for learning about containers, images, linux namespaces, cgroups, go lang, etc. Please make sure Docker is installed.

### Running Instructions
```
$ sudo su
# go build -o tinyc main.go
# ./tinyc run <image:tag> <command>
```

### Example
```
$ sudo ./tinyc run alpine sh
```
