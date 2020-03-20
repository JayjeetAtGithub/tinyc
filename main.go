package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

func must(err error) {
	// ensures no step went wrong
	// if err, then panic
	if err != nil {
		panic(err)
	}
}

func execute(command ...string) error {
	// execute the command
	// and return the error
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err
}

func prepareImage(image string) {
	// download the image from docker registry and
	// decompress it to a root file system
	err := execute("docker", "pull", image)
	if err != nil {
		log.Fatalf("Failed to pull image from Docker registry: %v\n", err.Error())
	}
	fmt.Printf("Pulled image: %v\n", image)
	convertImageToFS(image)
}

func convertImageToFS(image string) {
	// run a container from the image delegates the task of
	// merging the layers and generating the fs to docker
	runErr := execute("docker", "run", "--name", "tinyc", image)
	if runErr != nil {
		log.Fatalf("Failed to run the container: %v\n", runErr.Error())
	} else {
		// export the file system to a tar archive
		exportErr := execute("docker", "export", "--output=tinyc.tar", "tinyc")
		if exportErr != nil {
			log.Fatalf("Failed to export container to tar: %v\n", exportErr.Error())
		}
		os.Mkdir("/home/tinycfs", 0700)

		// extract the file system tar archive
		tarErr := execute("tar", "-C", "/home/tinycfs", "-xf", "tinyc.tar")
		if tarErr != nil {
			log.Fatalf("Failed to unarchive the fs tar: %v\n", tarErr.Error())
		}

		// remove the intermediate container and .tar archives
		rmErr := execute("docker", "rm", "-f", "tinyc")
		if rmErr != nil {
			log.Fatalf("Failed to remove the intermediate container: %v\n", rmErr.Error())
		}
		os.Remove("tinyc.tar")
	}
}

func setupEnvironment() {
	// set $USER, $HOME environment variables
	if os.Getuid() == 0 {
		must(os.Setenv("USER", "root"))
		must(os.Setenv("HOME", "/root"))
	} else {
		user, err := user.Current()
		if err != nil {
			log.Fatalf("Failed to read the current user: %v\n", err.Error())
		}
		must(os.Setenv("USER", user.Username))
		must(os.Setenv("HOME", filepath.Join("/home/", user.Username)))
	}

	// set the hostname, $PS1, $PATH environment variables
	must(syscall.Sethostname([]byte("container.local")))
	must(os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"))
}

func setupCgroups() {
	// set the control groups
	pid := os.Getpid()
	cgroups := "/sys/fs/cgroup"
	memoryCgroup := filepath.Join(cgroups, "memory")
	cpuCgroup := filepath.Join(cgroups, "cpu")

	os.Mkdir(filepath.Join(memoryCgroup, "container"), 0755)
	os.Mkdir(filepath.Join(cpuCgroup, "container"), 0755)

	// limit the container process to using 2 Mb memory only
	must(ioutil.WriteFile(filepath.Join(memoryCgroup, "container/memory.limit_in_bytes"),
		[]byte("2000000"), 0700))
	must(ioutil.WriteFile(filepath.Join(memoryCgroup, "container/notify_on_release"),
		[]byte("1"), 0700))
	must(ioutil.WriteFile(filepath.Join(memoryCgroup, "container/cgroup.procs"),
		[]byte(strconv.Itoa(pid)), 0700))

	// limit the container process to using 1 CPU only
	must(ioutil.WriteFile(filepath.Join(cpuCgroup, "container/cpu.shares"),
		[]byte("1"), 0700))
	must(ioutil.WriteFile(filepath.Join(cpuCgroup, "container/notify_on_release"),
		[]byte("1"), 0700))
	must(ioutil.WriteFile(filepath.Join(cpuCgroup, "container/cgroup.procs"),
		[]byte(strconv.Itoa(pid)), 0700))
}

func run(command ...string) {
	// run the container process in new namespaces
	cmd := exec.Command("/proc/self/exe", append([]string{"container"}, command[0:]...)...)

	// attach attributes for namespaces, UID-GID mappings
	// to the container process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNS,
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWPID,
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

	// bind the proc's STDIN, STDOUT, STDERR to the os's STDIN, STDOUT, STDERR
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to spawn the container process: %v\n", err.Error())
	}
}

func container(command ...string) {
	// set the control groups
	setupCgroups()

	// set the environment vars
	setupEnvironment()

	// chroot to the new root file system
	newRoot := "/home/tinycfs"
	chrootErr := syscall.Chroot(newRoot)
	if chrootErr != nil {
		log.Fatalf("Failed to chroot into the new file system: %v\n", chrootErr.Error())
	}

	// chdir into the new root file system
	must(os.Chdir("/"))

	log.Println("Group: ", os.Getgid())
	log.Println("User: ", os.Getuid())

	// make the necessary mounts
	must(syscall.Mount("proc", "proc", "proc", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""))
	must(syscall.Mount("tmpfs", "tmp", "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""))
	must(syscall.Mount("sysfs", "sys", "sysfs", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""))

	// execute the command in the container process
	log.Println("Executing: ", command)
	execErr := execute(command...)
	if execErr != nil {
		log.Fatalf("Failed to execute the command inside the container: %v\n", execErr.Error())
	}

	// unmount the mounts
	must(syscall.Unmount("proc", 0))
	must(syscall.Unmount("tmp", 0))
	must(syscall.Unmount("sys", 0))
}

func main() {
	// enforce executing as root user
	user, err := user.Current()
	if err != nil {
		log.Fatalf("Failed to fetch current user: %v\n", err.Error())
	}

	if user.Username != "root" {
		log.Fatalf("tinyc must be executed as a root user. Exiting.\n")
	}

	// if less then 4 arguments are provided,
	// show usage details and exit
	if len(os.Args) < 3 {
		fmt.Println("usage: ./main run <image> <cmd>")
		os.Exit(0)
	}
	// driver function
	switch os.Args[1] {
	case "run":
		prepareImage(os.Args[2])
		run(os.Args[3:]...)

	case "container":
		container(os.Args[2:]...)

	case "default":
		log.Fatalln("Unknown command.")
	}
}
