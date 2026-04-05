package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

func parseDockerRows(out, prefix string, minParts int) [][]string {
	if strings.TrimSpace(out) == "" {
		return [][]string{}
	}

	rows := make([][]string, 0)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < minParts || !strings.HasPrefix(parts[1], prefix) {
			continue
		}
		rows = append(rows, parts)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i][1] < rows[j][1]
	})

	return rows
}
