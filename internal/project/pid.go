package project

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func findPidByPort(port int) int {
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%d", port))
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) == 0 {
		return 0
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			// 检查是否 LISTENING 状态
			if strings.Contains(fields[3], "LISTENING") {
				pid, err := strconv.Atoi(strings.TrimSpace(fields[len(fields)-1]))
				if err == nil && pid > 0 {
					return pid
				}
			}
		}
	}
	return 0
}

func findPidByName(name string) int {
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("name='%s'", name), "get", "processid", "/value")
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) == 0 {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ProcessId=") {
			pidStr := strings.TrimPrefix(line, "ProcessId=")
			pid, err := strconv.Atoi(pidStr)
			if err == nil && pid > 0 {
				return pid
			}
		}
	}
	return 0
}
