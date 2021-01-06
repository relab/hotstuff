package profiling

import (
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"

	"github.com/felixge/fgprof"
)

func StartProfilers(cpuProfilePath, memProfilePath, tracePath, fgprofPath string) (stopProfile func() error, err error) {
	var (
		cpuProfile    *os.File
		traceFile     *os.File
		fgprofProfile *os.File
		fgprofStop    func() error
	)

	if cpuProfilePath != "" {
		cpuProfile, err = os.Create(cpuProfilePath)
		if err != nil {
			return nil, err
		}
		if err := pprof.StartCPUProfile(cpuProfile); err != nil {
			return nil, err
		}
	}

	if fgprofPath != "" {
		fgprofProfile, err = os.Create(fgprofPath)
		if err != nil {
			return nil, err
		}
		fgprofStop = fgprof.Start(fgprofProfile, fgprof.FormatPprof)
	}

	if tracePath != "" {
		traceFile, err = os.Create(tracePath)
		if err != nil {
			return nil, err
		}
		if err := trace.Start(traceFile); err != nil {
			return nil, err
		}
	}

	return func() error {
		// write memory profile
		if memProfilePath != "" {
			f, err := os.Create(memProfilePath)
			if err != nil {
				return err
			}
			runtime.GC() // get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				return err
			}
			f.Close()
		}

		if cpuProfile != nil {
			// stop cpu profile
			pprof.StopCPUProfile()
			err = cpuProfile.Close()
			if err != nil {
				return err
			}
		}

		// stop fgprof profile
		if fgprofProfile != nil {
			err = fgprofStop()
			if err != nil {
				return err
			}
			err = fgprofProfile.Close()
			if err != nil {
				return err
			}
		}

		if traceFile != nil {
			// stop trace
			trace.Stop()
			err = traceFile.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}, nil
}
