package src

import (
	"fmt"
	"strconv"
	"strings"
)

func nextContainerName(prefix string, existing []ContainerInfo) string {
	used := map[int]bool{}
	for _, c := range existing {
		suffix := strings.TrimPrefix(c.Name, prefix+"-")
		n, err := strconv.Atoi(suffix)
		if err == nil {
			used[n] = true
		}
	}
	for i := 1; ; i++ {
		if !used[i] {
			return fmt.Sprintf("%s-%d", prefix, i)
		}
	}
}
