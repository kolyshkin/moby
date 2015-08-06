## Docker ploop graphdriver

This document describes how to run the (*currently very experimental*) Docker ploop graphdriver.

Ploop is the enhanced block loop device, currently only available in
[OpenVZ](https://openvz.org/) kernel. To know more about ploop, see
[openvz.org/Ploop](https://openvz.org/Ploop).

This driver relies on the kernel ploop driver, a ploop C library and a goploop
library wrapper. It also uses shared deltas (via hardlinks), a configuration
not officially supported by libploop (but apparently it works fine).

### Prerequisites

To try the Docker ploop graphdriver, you need to have VZ7 installed,
see [openvz.org/Quick_install](https://openvz.org/Quick_install)).
Technically, any system with ploop kernel driver and working Docker
will do, but currently it appears that VZ7 is the only such system.

Once VZ7 is up and running, you need to install docker:
```bash
yum install docker
service docker start
```

### Building

Next, download my docker fork from github, and switch to ploop branch:
```bash
yum install git
git clone https://github.com/kolyshkin/docker
cd docker
git checkout ploop
```

All the following commands assume you are in docker git root directory.

Next, try to build the beast:
```bash
make dynbinary
```

### Using

If the above works (takes 10-15 minutes for the first time, consecutive
runs are faster thanks to caching), you can try starting it:

```bash
service docker stop
mv /var/lib/docker /var/lib/docker-orig
mkdir /var/lib/docker-ploop
ln -s /var/lib/docker-ploop /var/lib/docker
./bundles/1.7.1/dynbinary/docker-1.7.1 -D -d -s ploop # -d for daemon, -D for debug, -s to use ploop gd
```

Now you can try some docker commands.
The fastest one is probably this:
```bash
docker run busybox ps
```

You might notice docker daemon complains about incompatible
client version. You can fix it, too.
```bash
export PATH=`pwd`:$PATH
hash -r
which docker # make sure it shows the one you built
```

For something slower, you can try rebuilding yourself:
```bash
make dynbinary
```

### Escaping

If something went wrong, you need to switch back to stock Docker.
```bash
killall -TERM docker # stop the daemon
rm -f /var/lib/docker # rm the symlink
ln -s /var/lib/docker-orig /var/lib/docker
rm -rf bundles
hash -r
service docker start
```

### Initial results

Some very limited non-scientific tests shows that the driver in it current form
has a performance and disk space footprint similar to that of dm-thinp on loop device.

### Limitations

* The driver is work in progress: experimental, not optimized for speed, and probably very buggy (this is my first Go project, not counting goploop).
* Ploop dynamic resize is not used, all images and containers are of the same size (frankly I don't know if Docker has a concept of a per-container disk space limit).

### TODO

* Fix the FIXMEs
* Do the TODOs
* Per-ploop image locking
* Protect Get/Set and Create/Remove by mutexes
* Move clone code to goploop (or even libploop)
* Test
* Performance comparison
* Implement "changed files" tracker to optimize Diff()/Changes()

## See also
* [openvz.org](https://openvz.org)
* [openvz.org/Ploop](https://openvz.org/Ploop)
* [C ploop library](https://github.com/kolyshkin/ploop)
* [Go ploop library](https://github.com/kolyshkin/goploop) (a wrapper around C lib)
