// +build linux

package ploop

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/pkg/units"
	"github.com/docker/libcontainer/label"
	"github.com/kolyshkin/goploop"
)

const (
	imagePrefix = "root.hdd"
)

func init() {
	log.Debugf("[ploop] init")
	graphdriver.Register("ploop", Init)
}

type mount struct {
	count  int
	device string
}

type Driver struct {
	home    string
	master  string
	size    uint64
	mode    ploop.ImageMode
	clog    uint
	mountsM sync.RWMutex
	mounts  map[string]*mount
}

func Init(home string, opt []string) (graphdriver.Driver, error) {
	log.Debugf("[ploop] Init(home=%s)", home)

	// defaults
	m := ploop.Expanded
	var s int64 = 8589934592 // 8GiB
	var cl int64 = 9         // 9 is for 256K cluster block, 11 for 1M etc.

	for _, option := range opt {
		key, val, err := parsers.ParseKeyValueOpt(option)
		if err != nil {
			return nil, err
		}
		key = strings.ToLower(key)
		switch key {
		case "ploop.size":
			s, err = units.RAMInBytes(val)
			if err != nil {
				log.Errorf("[ploop] Bad value for ploop.size: %s", val)
				return nil, err
			}
		case "ploop.mode":
			m, err = ploop.ParseImageMode(val)
			if err != nil {
				log.Errorf("[ploop] Bad value for ploop.mode: %s", val)
				return nil, err
			}
		case "ploop.clog":
			cl, err = strconv.ParseInt(val, 10, 8)
			if err != nil || cl < 6 || cl > 16 {
				return nil, fmt.Errorf("[ploop] Bad value for ploop.clog: %s", val)
			}
		case "ploop.libdebug":
			libDebug, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("[ploop] Bad value for ploop.libdebug: %s", val)
			}
			ploop.SetVerboseLevel(libDebug)
		default:
			return nil, fmt.Errorf("[ploop] Unknown option %s", key)
		}
	}

	d := &Driver{
		home:   home,
		master: path.Join(home, "master"),
		mode:   m,
		size:   uint64(s >> 10), // convert to KB
		clog:   uint(cl),
		mounts: make(map[string]*mount),
	}

	// Remove old master image as image params might have changed,
	// ignoring the error if it's not there
	d.removeMaster(true)

	// create needed base dirs so we don't have to use MkdirAll() later
	dirs := []string{d.dir(""), d.mnt(""), d.master}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	// Create a new master image
	file := path.Join(d.master, imagePrefix)
	cp := ploop.CreateParam{Size: d.size, Mode: d.mode, File: file, CLog: d.clog, Flags: ploop.NoLazy}

	if err := ploop.Create(&cp); err != nil {
		log.Errorf("Can't create ploop image! Maybe some prerequisites are not met?")
		log.Errorf("Make sure you have ext4 filesystem in %s.", home)
		log.Errorf("Check that e2fsprogs and parted are installed.")
		return nil, err
	}

	return graphdriver.NaiveDiffDriver(d), nil
}

func (d *Driver) String() string {
	return "ploop"
}

func (d *Driver) Status() [][2]string {
	var buf syscall.Statfs_t
	syscall.Statfs(d.home, &buf)
	bs := uint64(buf.Bsize)
	total := buf.Blocks * bs
	free := buf.Bfree * bs
	used := (buf.Blocks - buf.Bfree) * bs

	d.mountsM.RLock()
	devCount := len(d.mounts)
	var devices string
	for _, m := range d.mounts {
		devices = devices + " " + m.device[5:]
	}
	d.mountsM.RUnlock()

	status := [][2]string{
		{"Home directory", d.home},
		{"Ploop mode", d.mode.String()},
		{"Ploop image size", units.BytesSize(float64(d.size << 10))},
		{"Disk space used", units.BytesSize(float64(used))},
		{"Disk space total", units.BytesSize(float64(total))},
		{"Disk space available", units.BytesSize(float64(free))},
		{"Active device count", strconv.Itoa(devCount)},
		{"Active devices", devices},
		/*
			{"Total images", xxx},
			{"Mounted devices", xxx},
		*/
	}

	return status
}

// this is for Docker 1.8 only
func (d *Driver) GetMetadata(id string) (map[string]string, error) {
	log.Debugf("[ploop] GetMetadata(id=%s)", id)

	var metadata map[string]string

	// TODO: return info about this ploop image:
	// format, size, mount point, block size, etc.
	// Maybe also: number of deltas etc.
	metadata["Format"] = "ploop"

	return metadata, nil
}

func (d *Driver) removeMaster(ignoreOpenError bool) {
	// Master image might be mounted
	p, err := ploop.Open(path.Join(d.master, ddxml))
	if err == nil {
		if m, _ := p.IsMounted(); m {
			p.Umount() // ignore errors
		}
		p.Close()
	} else if !ignoreOpenError {
		log.Warn(err)
	}
	// Remove master image
	if err := os.RemoveAll(d.master); err != nil {
		log.Warn(err) // might not be fatal but worth reporting
	}
}

func (d *Driver) Cleanup() error {
	log.Debugf("[ploop] Cleanup()")

	d.removeMaster(false)

	d.mountsM.Lock()
	for id, m := range d.mounts {
		log.Warnf("[ploop] Cleanup: unxpected ploop device %s, unmounting", m.device)
		if err := ploop.UmountByDevice(m.device); err != nil {
			log.Warnf("[ploop] Cleanup: %s", err)
		}
		delete(d.mounts, id)
	}
	d.mountsM.Unlock()

	return nil
}

func (d *Driver) create(id string) error {
	return copyDir(d.master, d.dir(id))
}

// add some info to our parent
func markParent(id, parent, dir, pdir string) error {
	// 1 symlink us to parent, just for the sake of debugging
	rpdir := path.Join("..", parent)
	if err := os.Symlink(rpdir, path.Join(dir, "parent")); err != nil {
		log.Errorf("[ploop] markParent: %s", err)
		return err
	}

	return nil
}

// clone creates a copy of a parent ploop
func (d *Driver) clone(id, parent string) error {
	dd := d.dd(id)
	dir := d.dir(id)
	pdd := d.dd(parent)
	pdir := d.dir(parent)

	// FIXME: lock parent delta!!

	// see if we can reuse a snapshot
	snap, err := readVal(pdir, "uuid-for-children")
	if err != nil {
		log.Errorf("[ploop] clone(): readVal: %s", err)
		return err
	}
	if snap == "" {
		// create a snapshot
		log.Debugf("[ploop] clone(): creating snapshot for %s", id)
		pp, err := ploop.Open(pdd)
		if err != nil {
			return err
		}

		snap, err = pp.Snapshot()
		if err != nil {
			pp.Close()
			return err
		}

		pp.Close() // save dd.xml now!

		// save snapshot for future children
		writeVal(pdir, "uuid-for-children", snap)
	} else {
		log.Debugf("[ploop] clone(): reusing snapshot %s from %s", snap, id)
	}

	markParent(id, parent, dir, pdir)

	// copy dd.xml from parent dir
	if err := copyFile(pdd, dd); err != nil {
		return err
	}

	// hardlink images from parent dir
	files, err := ioutil.ReadDir(pdir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		name := fi.Name()
		// TODO: maybe filter out non-files
		if !strings.HasPrefix(name, imagePrefix) {
			//			log.Debugf("[ploop] clone: skip %s", name)
			continue
		}
		src := path.Join(pdir, name)
		dst := path.Join(dir, name)
		//		log.Debugf("[ploop] clone: hardlink %s", name)
		if err = os.Link(src, dst); err != nil {
			return err
		}
	}

	// switch to snapshot, removing old top delta
	p, err := ploop.Open(dd)
	if err != nil {
		return err
	}
	defer p.Close()

	log.Debugf("[ploop] id=%s SwitchSnapshot(%s)", id, snap)
	if err = p.SwitchSnapshot(snap); err != nil {
		return err
	}

	return nil
}

func (d *Driver) Create(id, parent string) error {
	log.Debugf("[ploop] Create(id=%s, parent=%s)", id, parent)

	// Assuming Create is called for non-existing stuff only
	dir := d.dir(id)
	err := os.Mkdir(dir, 0700)
	if err != nil {
		return err
	}

	if parent == "" {
		err = d.create(id)
	} else {
		err = d.clone(id, parent)
	}

	if err != nil {
		os.RemoveAll(dir)
		return err
	}

	// Make sure the mount point exists
	mdir := d.mnt(id)
	err = os.Mkdir(mdir, 0755)
	if err != nil {
		return err
	}

	return nil
}

func (d *Driver) Remove(id string) error {
	log.Debugf("[ploop] Remove(id=%s)", id)

	// Check if ploop was properly Get/Put:ed and is therefore unmounted
again:
	d.mountsM.Lock()
	_, ok := d.mounts[id]
	d.mountsM.Unlock()
	if ok {
		log.Warnf("[ploop] Remove(id=%s): unexpected on non-Put()", id)
		d.Put(id)
		goto again
	}

	dirs := []string{d.dir(id), d.mnt(id)}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) Get(id, mountLabel string) (string, error) {
	mnt := d.mnt(id)

	d.mountsM.Lock()
	defer d.mountsM.Unlock()
	m, ok := d.mounts[id]
	if ok {
		if m.count > 0 {
			m.count++
			log.Debugf("[ploop] skip Get(id=%s), dev=%s, count=%d", id, m.device, m.count)
			return mnt, nil
		} else {
			log.Warnf("[ploop] Get() id=%s, dev=%s: unexpected count=%d", id, m.device, m.count)
		}
	}

	log.Debugf("[ploop] Get(id=%s)", id)
	var mp ploop.MountParam

	dd := d.dd(id)
	dir := d.dir(id)
	mp.Target = mnt
	mp.Data = label.FormatMountLabel("", mountLabel)

	// Open ploop descriptor
	p, err := ploop.Open(dd)
	if err != nil {
		return "", err
	}
	defer p.Close()

	_, err = os.Stat(path.Join(dir, "uuid-for-children"))
	if err == nil {
		// This snapshot was already used to clone children from,
		// so we assume it won't be modified and mount it read-only.
		// If this assumption is not true (i.e. write access is needed)
		// we need to invalidate the snapshot by calling
		//	removeVal(dir, "uuid-for-children")
		// and then we can mount it read/write without fear.
		mp.Readonly = true
	} else if !os.IsNotExist(err) {
		log.Warnf("[ploop] Unexpected error: %s", err)
	}

	// Mount
	dev, err := p.Mount(&mp)
	if err != nil {
		return "", err
	}

	d.mounts[id] = &mount{1, dev}

	return mnt, nil
}

func (d *Driver) Put(id string) error {
	d.mountsM.Lock()
	defer d.mountsM.Unlock()
	m, ok := d.mounts[id]
	if ok {
		if m.count > 1 {
			m.count--
			log.Debugf("[ploop] skip Put(id=%s), dev=%s, count=%d", id, m.device, m.count)
			return nil
		} else if m.count < 1 {
			log.Warnf("[ploop] Put(id=%s): unexpected mount count %d", m.count)
		}
	}

	log.Debugf("[ploop] Put(id=%s)", id)

	dd := d.dd(id)
	p, err := ploop.Open(dd)
	if err != nil {
		return err
	}
	defer p.Close()

	err = p.Umount()
	/* Ignore "not mounted" error */
	if ploop.IsNotMounted(err) {
		err = nil
	}
	delete(d.mounts, id)
	return err
}

func (d *Driver) Exists(id string) bool {
	log.Debugf("[ploop] Exists(id=%s)", id)

	// Check if DiskDescriptor.xml is there
	dd := d.dd(id)
	_, err := os.Stat(dd)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Errorf("[ploop] Unexpected error from stat(): %s", err)
		}
		return false
	}

	return true
}
