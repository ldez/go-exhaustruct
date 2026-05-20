package analyzer

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"

	"dev.gaijin.team/go/golib/e"
	"dev.gaijin.team/go/golib/fields"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"dev.gaijin.team/go/exhaustruct/v5/internal/astutil"
	"dev.gaijin.team/go/exhaustruct/v5/internal/directive"
	"dev.gaijin.team/go/exhaustruct/v5/internal/pattern"
	"dev.gaijin.team/go/exhaustruct/v5/internal/structure"
)

func NewAnalyzerWithConfig(config *Config) (*analysis.Analyzer, error) {
	processor, err := newProcessor(config)
	if err != nil {
		return nil, err
	}

	a := newBaseAnalyzer()

	a.Run = func(pass *analysis.Pass) (any, error) {
		run(pass, config, processor)

		return nil, nil //nolint:nilnil
	}

	return a, nil
}

func NewAnalyzer() *analysis.Analyzer {
	config := &Config{}

	a := newBaseAnalyzer()

	lazyProcessor := sync.OnceValues(func() (*structure.Processor, error) {
		return newProcessor(config)
	})

	a.Run = func(pass *analysis.Pass) (any, error) {
		processor, err := lazyProcessor()
		if err != nil {
			return nil, err
		}

		run(pass, config, processor)

		if config.DebugCacheMetrics {
			pkgPath := pass.Pkg.Path()

			siHits, siMisses, siSize := processor.Stats()
			printCacheLine(pkgPath, "struct-infos", siHits, siMisses, siSize)

			fdHits, fdMisses, fdSize := processor.Directives().Stats()
			printCacheLine(pkgPath, "file-directives", fdHits, fdMisses, fdSize)

			printMemStats(pkgPath)
		}

		return nil, nil //nolint:nilnil
	}

	a.Flags.Init("", flag.PanicOnError)

	bindToFlagSet(&a.Flags, config)

	return a
}

func newBaseAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{ //nolint:exhaustruct
		Name:     "exhaustruct",
		Doc:      "Checks if all structure fields are initialized",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
}

func run(pass *analysis.Pass, config *Config, processor *structure.Processor) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector) //nolint:forcetypeassert

	for _, diag := range processor.Directives().ProcessFiles(pass.Fset, pass.Files...) {
		pass.Report(diag)
	}

	newMissingFieldsVisitor(config, processor).run(pass, insp)
	newTagMigrationVisitor().run(pass, insp)
}

func printMemStats(pkgPath string) {
	var m runtime.MemStats

	runtime.ReadMemStats(&m)

	const mb = 1024 * 1024

	_, _ = fmt.Fprintf(os.Stderr, "[%s] memory: alloc=%dMB sys=%dMB heap=%dMB\n",
		pkgPath, m.Alloc/mb, m.Sys/mb, m.HeapAlloc/mb)
}

func printCacheLine(pkgPath, name string, hits, misses, size uint64) {
	hitRate := float64(0)
	if total := hits + misses; total > 0 {
		hitRate = float64(hits) / float64(total) * 100 //nolint:mnd
	}

	_, _ = fmt.Fprintf(os.Stderr, "[%s] cache: %s: hits=%d misses=%d size=%d (%.2f%%)\n",
		pkgPath, name, hits, misses, size, hitRate)
}

func newProcessor(config *Config) (*structure.Processor, error) {
	enforce, err := pattern.NewList(config.EnforcePatterns...)
	if err != nil {
		return nil, e.NewFrom("compile enforce patterns", err, fields.F("flag", "enforce-rx"))
	}

	ignore, err := pattern.NewList(config.IgnorePatterns...)
	if err != nil {
		return nil, e.NewFrom("compile ignore patterns", err, fields.F("flag", "ignore-rx"))
	}

	optional, err := pattern.NewList(config.OptionalPatterns...)
	if err != nil {
		return nil, e.NewFrom("compile optional patterns", err, fields.F("flag", "optional-rx"))
	}

	allowEmpty, err := pattern.NewList(config.AllowEmptyPatterns...)
	if err != nil {
		return nil, e.NewFrom("compile allow-empty patterns", err, fields.F("flag", "allow-empty-rx"))
	}

	fp := astutil.NewFileParser()

	return structure.NewProcessor(
		directive.NewScanner(fp),
		structure.NewOriginScanner(fp),
		structure.WithEnforce(enforce),
		structure.WithIgnore(ignore),
		structure.WithOptional(optional),
		structure.WithAllowEmpty(allowEmpty),
	), nil
}
