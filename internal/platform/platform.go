package platform

import "runtime"

// Current returns GOOS/GOARCH pair.
func Current() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
