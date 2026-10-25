//go:build android

package debug

import (
	"context"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/host"
)

// AddPlatformInfo reports Go runtime and kernel info; build properties are on the Java side.
func (di *Info) AddPlatformInfo(ctx context.Context) {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		di.AddSection(
			"Platform Information",
			NoFlags,
			fmt.Sprintf("Failed to get: %s", err),
		)
		return
	}

	di.AddSection(
		fmt.Sprintf("Platform: Android %s", info.PlatformVersion),
		UseCodeSection|AddContentLineBreaks,
		fmt.Sprintf("System: %s %s (%s)", info.Platform, info.OS, info.PlatformFamily),
		fmt.Sprintf("Kernel: %s %s", info.KernelVersion, info.KernelArch),
		fmt.Sprintf("Runtime: %s/%s", runtime.GOOS, runtime.GOARCH),
	)
}
