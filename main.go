package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

func check(err error) {
	// if err, panic
	if err != nil {
		panic(err)
	}
}

func setupEnvironment() {
	// Set USER, HOME environment variables
	if os.Getuid() == 0 {
		check(os.Setenv("USER", "root"))
		check(os.Setenv("HOME", "/root"))
	} else {
		u, err := user.Current()
		check(err)
		check(os.Setenv("USER", u.Username))
		check(os.Setenv("HOME", filepath.Join("/home/", u.Username)))
	}

	// Set the hostnames, PS1, PATH environment variables
	check(syscall.Sethostname([]byte("container.local")))
	check(os.Setenv("PS1", "$USER@$HOSTNAME:$PWD~$ "))
	check(os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"))
}

func setupCgroups() {
	// Set the CGroups
	pid := os.Getpid()
	cgroups := "/sys/fs/cgroup"
	memoryCgroup := filepath.Join(cgroups, "memory")
	cpuCgroup := filepath.Join(cgroups, "cpu")

	os.Mkdir(filepath.Join(memoryCgroup, "container"), 0755)
	os.Mkdir(filepath.Join(cpuCgroup, "container"), 0755)

	// Limit to 2 Mb memory only
	check(ioutil.WriteFile(filepath.Join(memoryCgroup, "container/memory.limit_in_bytes"),
		[]byte("2000000"), 0700))
	check(ioutil.WriteFile(filepath.Join(memoryCgroup, "container/notify_on_release"),
		[]byte("1"), 0700))
	check(ioutil.WriteFile(filepath.Join(memoryCgroup, "container/cgroup.procs"),
		[]byte(strconv.Itoa(pid)), 0700))

	// Limit to 1 CPU only
	check(ioutil.WriteFile(filepath.Join(cpuCgroup, "container/cpu.shares"),
		[]byte("1"), 0700))
	check(ioutil.WriteFile(filepath.Join(cpuCgroup, "container/notify_on_release"),
		[]byte("1"), 0700))
	check(ioutil.WriteFile(filepath.Join(cpuCgroup, "container/cgroup.procs"),
		[]byte(strconv.Itoa(pid)), 0700))
}

func main() {
	switch os.Args[1] {
	case "run":
		run(os.Args[2:]...)

	case "container":
		container(os.Args[2:]...)

	case "default":
		log.Fatal("Unknown command.")
	}
}

func run(command ...string) {
	// Run the container process in new namespaces
	cmd := exec.Command("/proc/self/exe", append([]string{"container"}, command[0:]...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWPID,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	check(cmd.Run())
}

func container(command ...string) {
	// Set the control groups
	setupCgroups()

	// Set the environment vars
	setupEnvironment()

	// Chroot into the root file system
	newRoot := "./fs"
	oldRoot := filepath.Join(newRoot, ".oldroot")
        check(os.MkdirAll(oldRoot, 0700))

	err_1 := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, "")
	if err_1 != nil {
		log.Println("Error while mounting")
	}

	err_2 := syscall.PivotRoot(newRoot, oldRoot)
	if err_2 != nil {
		log.Println("Error while pivoting", err_2)
	}

	check(os.Chdir("/"))

	err_3 := syscall.Unmount("/.oldroot", syscall.MNT_DETACH)
	if err_3 != nil {
		log.Println("Error while unmounting", err_3)
	}

	err_4 := os.Remove("/.oldroot")
	if err_4 != nil {
		log.Println("Error while removing old root", err_4)
	}

	log.Println("GROUP: ", os.Getgid())
	log.Println("USER: ", os.Getuid())

	// Make the necessary mounts
	check(syscall.Mount("proc", "proc", "proc", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""))
	check(syscall.Mount("tmpfs", "tmp", "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""))
	check(syscall.Mount("sysfs", "sys", "sysfs", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""))

	// Execute the command in the container process
	log.Println("Executing: ", command[0])
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	check(cmd.Run())

	// Unmount the mounts
	check(syscall.Unmount("proc", 0))
	check(syscall.Unmount("tmp", 0))
	check(syscall.Unmount("sys", 0))
}
