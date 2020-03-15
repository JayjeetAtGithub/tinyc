package main

import (
	"fmt"
	"os"
	"log"
	"os/exec"
	"io/ioutil"
	"syscall"
	"path/filepath"
	"strconv"
)

func check(err error) {
	if err !=  nil {
		panic(err)
	}
}

func setCgroups() {
	cgroups := "/sys/fs/cgroup"
	mem := filepath.Join(cgroups, "memory")
	os.Mkdir(filepath.Join(mem, "container"), 0755)

	check(ioutil.WriteFile(filepath.Join(mem, "container/memory.limit_in_bytes"),
		[]byte("5000000"), 0700))
	check(ioutil.WriteFile(filepath.Join(mem, "container/notify_on_release"),
		[]byte("1"), 0700))

	pid := strconv.Itoa(os.Getpid())
	check(ioutil.WriteFile(filepath.Join(mem, "container/cgroup.procs"),
		[]byte(pid), 0700))
}

func main() {
	switch os.Args[1] {
	case "run":
		run(os.Args[2:]...)

	case "child":
		child(os.Args[2:]...)

	case "default":
		log.Fatal("Unknown command.")
	}
}

func run(command ...string) {
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, command[0:]...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID  |
			    syscall.CLONE_NEWNS   |
			    syscall.CLONE_NEWIPC  |
                            syscall.CLONE_NEWNET  |
		            syscall.CLONE_NEWUSER |
			    syscall.CLONE_NEWUTS,
		            UidMappings: []syscall.SysProcIDMap{
						{
							ContainerID: 0,
							HostID:      os.Getuid(),
							Size:        1,
						},
			    },
			    GidMappings: []syscall.SysProcIDMap{
						{
							ContainerID: 0,
							HostID:      os.Getgid(),
							Size:        1,
						},
			    },
	    }
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Println("ERROR: ", err)
		os.Exit(1)
	}
}

func child(command ...string) {
	setCgroups()

	check(os.Setenv("PS1", "root@container~#"))
	check(os.Setenv("HOME", "/"))
	check(os.Setenv("USER", "/root"))
	
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	check(syscall.Sethostname([]byte("container")))
	check(syscall.Chroot("./rootfs"))
	check(os.Chdir("./rootfs"))
	check(syscall.Mount("proc", "proc", "proc", 0, ""))
	check(syscall.Mount("tmp", "tmp", "tmpfs", 0, ""))

	err := cmd.Run()
	if err != nil {
		fmt.Println("ERROR: ", err)
		os.Exit(1)
	}

	check(syscall.Unmount("proc", 0))
	check(syscall.Unmount("tmp", 0))
}
