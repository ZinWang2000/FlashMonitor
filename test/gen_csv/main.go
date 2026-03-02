package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var sections = []string{".text", ".data", ".bss", ".rodata", ".debug", ".init", ".fini"}
var modules = []string{
	"core", "hal", "drivers", "net", "fs", "crypto", "ui", "utils", "platform", "rtos",
	"audio", "video", "sensors", "wireless", "storage", "power", "security", "codec",
}

func main() {
	rows := flag.Int("rows", 1000000, "Number of rows per CSV file")
	files := flag.Int("files", 100, "Number of distinct filenames")
	pkgs := flag.Int("packages", 2, "Number of packages (generates one CSV per package)")
	outDir := flag.String("out", ".", "Output directory")
	seed := flag.Int64("seed", time.Now().UnixNano(), "Random seed for row data")
	fseed := flag.Int64("fseed", 0, "Random seed for filename pool; 0 = same as -seed")
	flag.Parse()

	if *fseed == 0 {
		*fseed = *seed
	}

	fileRng := rand.New(rand.NewSource(*fseed))
	rng := rand.New(rand.NewSource(*seed))
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %q: %v\n", *outDir, err)
		os.Exit(1)
	}

	filePool := make([]string, *files)
	for i := range filePool {
		filePool[i] = fmt.Sprintf("%s/%s_obj_%04d.o",
			modules[fileRng.Intn(len(modules))],
			modules[fileRng.Intn(len(modules))],
			i)
	}

	for p := 0; p < *pkgs; p++ {
		pkgName := fmt.Sprintf("package_%c", 'A'+p)
		outPath := filepath.Join(*outDir, pkgName+".csv")
		start := time.Now()

		f, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %q: %v\n", outPath, err)
			os.Exit(1)
		}

		w := csv.NewWriter(f)
		_ = w.Write([]string{"Section", "ModuleName", "FileName", "Size", "MangledName"})

		for i := 0; i < *rows; i++ {
			section := sections[rng.Intn(len(sections))]
			module := modules[rng.Intn(len(modules))]
			fileName := filePool[rng.Intn(len(filePool))]
			size := rng.Intn(65536) + 1
			mangled := fmt.Sprintf("_Z%d%s%dv", rng.Intn(20)+2, module, rng.Intn(100))

			_ = w.Write([]string{section, module, fileName, fmt.Sprintf("%d", size), mangled})

			if i%100000 == 0 && i > 0 {
				w.Flush()
				fmt.Printf("\r  pkg %s: %d/%d rows...", pkgName, i, *rows)
			}
		}

		w.Flush()
		f.Close()

		elapsed := time.Since(start)
		fi, _ := os.Stat(outPath)
		fmt.Printf("\r  ✓ %s: %d rows, %.1f MB, %v\n",
			outPath, *rows, float64(fi.Size())/1e6, elapsed.Round(time.Millisecond))
	}

	fmt.Printf("\nDone. Generated %d CSV files in %q\n", *pkgs, *outDir)
}
