package platform

import (
	"runtime"
	"strings"
)

// Current returns GOOS/GOARCH pair.
func Current() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// RuntimeFileCandidates returns local runtime artifact names in preferred order.
func RuntimeFileCandidates(goos, fileName string) []string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return nil
	}

	candidates := []string{}
	appendUnique := func(candidate string) {
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if existing == candidate {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	lower := strings.ToLower(fileName)
	if goos == "windows" {
		if strings.HasSuffix(lower, ".llamafile") && !strings.HasSuffix(lower, ".exe") {
			appendUnique(fileName + ".exe")
		}
		appendUnique(fileName)
		if strings.HasSuffix(lower, ".llamafile.exe") {
			appendUnique(strings.TrimSuffix(fileName, ".exe"))
		}
		return candidates
	}

	appendUnique(fileName)
	return candidates
}

// PreferredRuntimeFileName returns the local artifact name to write on the current OS.
func PreferredRuntimeFileName(goos, fileName string) string {
	candidates := RuntimeFileCandidates(goos, fileName)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}
