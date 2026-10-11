//go:build darwin

package scheduler

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type userProcess struct {
	pid  int
	name string
}

func processesForLoggedInUser() ([]userProcess, error) {
	username := getMacUser()
	if username == "" && os.Getuid() != 0 {
		currentUser, err := user.Current()
		if err != nil {
			return nil, err
		}
		username = currentUser.Username
	}
	if username == "" {
		return nil, nil
	}

	output, err := exec.Command("/bin/ps", "-U", username, "-o", "pid=,comm=").Output()
	if err != nil {
		return nil, err
	}

	var processes []userProcess
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		processName := filepath.Base(strings.Join(fields[1:], " "))
		processes = append(processes, userProcess{pid: pid, name: processName})
	}
	return processes, nil
}

func findRunningAppProcesses(apps []string) []string {
	processes, err := processesForLoggedInUser()
	if err != nil {
		log.Printf("scheduler: list processes for app warning: %v", err)
		return nil
	}
	running := make(map[string]bool)
	for _, process := range processes {
		for _, app := range apps {
			if processNameMatchesApp(process.name, app) {
				running[app] = true
			}
		}
	}
	var runningApps []string
	for _, app := range apps {
		if running[app] {
			runningApps = append(runningApps, app)
		}
	}
	return runningApps
}

func terminateBlockedProcesses(apps []string) {
	if len(apps) == 0 {
		return
	}
	processes, err := processesForLoggedInUser()
	if err != nil {
		log.Printf("scheduler: list processes for termination: %v", err)
		return
	}
	for _, process := range processes {
		for _, app := range apps {
			if !processNameMatchesApp(process.name, app) {
				continue
			}
			if err := syscall.Kill(process.pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
				log.Printf("scheduler: terminate %s process %d: %v", process.name, process.pid, err)
			}
			break
		}
	}
}
