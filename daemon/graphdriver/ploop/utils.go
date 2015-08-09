package ploop

import (
	"io"
	"io/ioutil"
	"os"
	"path"
)

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

// copyDir copies a directory (non-recursively, i.e. only files
func copyDir(sdir, ddir string) (err error) {
	files, err := ioutil.ReadDir(sdir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		name := fi.Name()
		src := path.Join(sdir, name)
		dst := path.Join(ddir, name)
		// filter out non-files
		if !fi.Mode().IsRegular() {
			//			log.Warnf("[ploop] unexpected non-file %s", src)
			continue
		}
		if err = copyFile(src, dst); err != nil {
			return err
		}
	}

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
