package decoder

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

var debugMovieTraceSpec struct {
	once    sync.Once
	enabled bool
	targets map[[2]int]struct{}
}

func debugTraceMovieBlock(x, y, width, height int) bool {
	debugMovieTraceSpec.once.Do(func() {
		debugMovieTraceSpec.enabled = os.Getenv("DEBUG_MOVIE_TRACE_TARGET") != ""
		if !debugMovieTraceSpec.enabled {
			return
		}
		spec := strings.TrimSpace(os.Getenv("DEBUG_MOVIE_TRACE_BLOCK"))
		if spec == "" {
			debugMovieTraceSpec.targets = map[[2]int]struct{}{
				{832, 32}: {},
				{836, 32}: {},
			}
			return
		}
		debugMovieTraceSpec.targets = make(map[[2]int]struct{})
		for _, item := range strings.Split(spec, ";") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			parts := strings.Split(item, ",")
			if len(parts) != 2 {
				continue
			}
			tx, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
			ty, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errX != nil || errY != nil {
				continue
			}
			debugMovieTraceSpec.targets[[2]int{tx, ty}] = struct{}{}
		}
		if len(debugMovieTraceSpec.targets) == 0 {
			debugMovieTraceSpec.targets = map[[2]int]struct{}{
				{832, 32}: {},
				{836, 32}: {},
			}
		}
	})
	if !debugMovieTraceSpec.enabled {
		return false
	}
	if os.Getenv("DEBUG_MOVIE_TRACE_ALL") != "" {
		if os.Getenv("DEBUG_MOVIE_TRACE_ANY_SIZE") == "" && (width != 4 || height != 4) {
			return false
		}
		return true
	}
	if os.Getenv("DEBUG_MOVIE_TRACE_ANY_SIZE") == "" && (width != 4 || height != 4) {
		return false
	}
	_, ok := debugMovieTraceSpec.targets[[2]int{x, y}]
	return ok
}
