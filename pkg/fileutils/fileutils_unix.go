// +build linux freebsd

package fileutils // import "github.com/docker/docker/pkg/fileutils"

import (
	"os"

	"github.com/sirupsen/logrus"
)

// GetTotalUsedFds returns the number of file descriptors
// opened by the current process.
func GetTotalUsedFds() int {
	f, err := os.Open("/proc/self/fd")
	if err != nil {
		logrus.Error(err)
		return -1
	}

	list, err := f.Readdirnames(-1)
	if err != nil {
		logrus.Error(err)
		return -1
	}

	return len(list)
}
