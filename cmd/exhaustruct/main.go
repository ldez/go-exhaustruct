package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"dev.gaijin.team/go/exhaustruct/v5/analyzer"
)

func main() {
	singlechecker.Main(analyzer.NewAnalyzer())
}
