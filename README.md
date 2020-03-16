## A simple container implementation in Go

```
mkdir -p rootfs/
tar -C rootfs/ -xvf rootfs.tar
go build main.go
sudo ./main run /bin/sh
```
