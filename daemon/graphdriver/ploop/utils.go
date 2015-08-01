package ploop

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"syscall"
)

// isMountPoint determines if a directory is a mountpoint, by comparing the device for the
// directory with the device for it's parent.  If they are the same, it's not a mountpoint,
// if they're different, it is.
func IsMountPoint(file string) (bool, error) {
	stat, err := os.Stat(file)
	if err != nil {
		return false, err
	}
	rootStat, err := os.Lstat(file + "/..")
	if err != nil {
		return false, err
	}
	// If the directory has the same device as parent, then it's not a mountpoint.
	return stat.Sys().(*syscall.Stat_t).Dev != rootStat.Sys().(*syscall.Stat_t).Dev, nil
}

// copyFile copies a file (like command-line cp)
func copyFile(src, dst string) (err error) {
	s, err := os.Open(src)
	if err != nil {
		return
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := d.Close()
		if err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(d, s)
	if err != nil {
		os.Remove(dst)
		return
	}

	// TODO: chown/chmod maybe?
	return nil
}

func writeVal(dir, id, val string) error {
	m := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	fd, err := os.OpenFile(path.Join(dir, id), m, 0644)
	if err != nil {
		return err
	}

	_, err = fd.WriteString(val)
	if err != nil {
		return err
	}

	err = fd.Close()

	return err
}

func readVal(dir, id string) (string, error) {
	buf, err := ioutil.ReadFile(path.Join(dir, id))
	if err == nil {
		return string(buf), nil
	}

	if os.IsNotExist(err) {
		return "", nil
	}

	return "", err
}

func removeVal(dir, id string) error {
	return os.Remove(path.Join(dir, id))
}
