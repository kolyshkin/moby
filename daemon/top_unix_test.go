//+build !windows

package daemon // import "github.com/docker/docker/daemon"

import (
	"testing"
)

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
		_, err := parsePSOutput(f.output, f.pids, false)
		t.Logf("tested %q, got err=%v", string(f.output), err)
		if f.errExpected && err == nil {
			t.Fatalf("expected error, got %v (%q)", err, string(f.output))
		}
		if !f.errExpected && err != nil {
			t.Fatalf("expected nil, got %v (%q)", err, string(f.output))
		}
	}
}
