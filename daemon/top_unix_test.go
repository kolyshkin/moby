//+build !windows

package daemon // import "github.com/docker/docker/daemon"

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
)

func TestContainerTopValidatePSArgs(t *testing.T) {
	tests := map[string]bool{
		defaultPsArgs:               false,
		"ae -o uid=PID":             true,
		"ae -o \"uid= PID\"":        true,  // ascii space (0x20)
		"ae -o \"uid= PID\"":        false, // unicode space (U+2003, 0xe2 0x80 0x83)
		"ae o uid=PID":              true,
		"aeo uid=PID":               true,
		"ae -O uid=PID":             true,
		"ae -o pid=PID2 -o uid=PID": true,
		"ae -o pid=PID":             false,
		"ae -o pid=PID -o uid=PIDX": true, // FIXME: we do not need to prohibit this
		"aeo pid=PID":               false,
		"ae":                        false,
		"":                          false,
	}
	for psArgs, errExpected := range tests {
		err := validatePSArgs(psArgs)
		t.Logf("tested %q, got err=%v", psArgs, err)
		if errExpected && err == nil {
			t.Fatalf("expected error, got %v (%q)", err, psArgs)
		}
		if !errExpected && err != nil {
			t.Fatalf("expected nil, got %v (%q)", err, psArgs)
		}
	}
}

func TestContainerTopParsePSOutput(t *testing.T) {
	tests := []struct {
		output      []byte
		pids        []uint32
		errExpected bool
	}{
		{[]byte(`  PID COMMAND
   42 foo
   43 bar
		- -
  100 baz
`), []uint32{42, 43}, false},
		{[]byte(`  UID COMMAND
   42 foo
   43 bar
		- -
  100 baz
`), []uint32{42, 43}, true},
		// unicode space (U+2003, 0xe2 0x80 0x83)
		{[]byte(` PID COMMAND
   42 foo
   43 bar
		- -
  100 baz
`), []uint32{42, 43}, true},
		// the first space is U+2003, the second one is ascii.
		{[]byte(` PID COMMAND
   42 foo
   43 bar
  100 baz
`), []uint32{42, 43}, true},
	}

	for _, f := range tests {
		_, err := parsePSOutput(f.output, f.pids)
		t.Logf("tested %q, got err=%v", string(f.output), err)
		if f.errExpected && err == nil {
			t.Fatalf("expected error, got %v (%q)", err, string(f.output))
		}
		if !f.errExpected && err != nil {
			t.Fatalf("expected nil, got %v (%q)", err, string(f.output))
		}
	}
}

func BenchmarkParsePsOutput(b *testing.B) {
	const (
		pidMax  = 32768 // as in /proc/sys/kernel/pid_max
		psLines = 10000 // number of lines returned by ps, must be < pidMax
		pidNum  = 100   // number of container PIDs, must be < psLines
	)

	// Generate input data for parsePSOutput()
	rnd := rand.New(rand.NewSource(42))

	// template of ps output
	hdr := []byte("UID        PID  PPID  C STIME TTY          TIME CMD\n")
	line := []byte("kir       #pid# 4952  0 May25 pts/0    00:00:01 bash\n")
	out := append(hdr, bytes.Repeat(line, psLines)...)

	// container pids
	pids := make([]uint32, pidNum)
	var offset int
	for p := 0; p < pidNum; p++ {
		pid := uint32(rnd.Intn(pidMax))
		pids[p] = pid
		// replace a template by the pid
		i := bytes.IndexByte(out[offset:], '#')
		if i < 0 {
			b.Fatalf("not enough lines for %d pids", pidNum)
		}
		offset += i
		pidStr := fmt.Sprintf("%5d", pid)
		copy(out[offset:], pidStr)
	}

	// replace the rest of #pid# placeholders
	for {
		i := bytes.IndexByte(out[offset:], '#')
		if i < 0 {
			break // no more placeholders
		}
		offset += i
		copy(out[offset:], []byte("44444")) // replacement should be > pidMax
	}

	// benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := parsePSOutput(out, pids)
		if err != nil {
			b.Fatal(err)
		}
		// check number of lines returned is as expected
		if len(res.Processes) != pidNum {
			b.Fatalf("expected %d lines, got %d", pidNum, len(res.Processes))
		}
	}
}
