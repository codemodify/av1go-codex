package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/codemodify/av1go-codex/internal/msac"
)

var debugGoBlockTraceSpec struct {
	once    sync.Once
	targets map[[2]int]struct{}
}

func debugBlockTraceEnabled(g BlockGeometry) bool {
	if os.Getenv("DEBUG_GO_BLOCK_TRACE") == "" {
		return false
	}
	debugGoBlockTraceSpec.once.Do(func() {
		debugGoBlockTraceSpec.targets = map[[2]int]struct{}{{0, 0}: {}}
		spec := strings.TrimSpace(os.Getenv("DEBUG_GO_BLOCK_TRACE_BLOCK"))
		if spec == "" {
			return
		}
		targets := make(map[[2]int]struct{})
		for _, item := range strings.Split(spec, ";") {
			parts := strings.Split(strings.TrimSpace(item), ",")
			if len(parts) != 2 {
				continue
			}
			x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
			y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errX != nil || errY != nil {
				continue
			}
			targets[[2]int{x, y}] = struct{}{}
		}
		if len(targets) != 0 {
			debugGoBlockTraceSpec.targets = targets
		}
	})
	_, ok := debugGoBlockTraceSpec.targets[[2]int{g.Start4X * 4, g.Start4Y * 4}]
	return ok
}

func debugTraceEntropy(label string, dec *msac.Context, format string, args ...any) {
	if dec == nil {
		return
	}
	cur, rng, cnt, pos := dec.DebugState()
	fmt.Fprintf(os.Stderr, "go-trace %s state=(%d,%d,%d,%d)", label, cur, rng, cnt, pos)
	if format != "" {
		fmt.Fprint(os.Stderr, " ")
		fmt.Fprintf(os.Stderr, format, args...)
	}
	fmt.Fprintln(os.Stderr)
}
