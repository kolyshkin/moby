package loggerutils

import (
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/daemon/logger"
	"gotest.tools/assert"
)

func TestFollowLogsClose(t *testing.T) {
	lw := logger.NewLogWatcher()

	f, err := ioutil.TempFile("", t.Name())
	assert.NilError(t, err)
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	makeDecoder := func(rdr io.Reader) func() (*logger.Message, error) {
		return func() (*logger.Message, error) {
			return &logger.Message{}, nil
		}
	}

	followLogsDone := make(chan struct{})
	var since, until time.Time
	go func() {
		followLogs(f, lw, make(chan interface{}), makeDecoder, since, until)
		close(followLogsDone)
	}()

	select {
	case <-lw.Msg:
	case err := <-lw.Err:
		assert.NilError(t, err)
	case <-followLogsDone:
		t.Fatal("follow logs finished unexpectedly")
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for log message")
	}

	lw.Close()
	select {
	case <-followLogsDone:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for followLogs() to finish")
	}
}
