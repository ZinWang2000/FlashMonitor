BINARY    = flashmonitor.exe
LIBS_DIR  = $(shell pwd)/libs
MINGW_BIN = /c/msys64/mingw64/bin

.PHONY: build build-stub test clean

# Build the seekpos ABI-fix stub (only needed once, or when libs/ is missing)
build-stub:
	@echo "Building seekpos ABI-fix stub..."
	$(MINGW_BIN)/as.exe -o $(LIBS_DIR)/seekpos_fix.o $(LIBS_DIR)/seekpos_fix.s
	$(MINGW_BIN)/ar.exe rcs $(LIBS_DIR)/libseekpos_fix.a $(LIBS_DIR)/seekpos_fix.o
	@echo "Stub built: $(LIBS_DIR)/libseekpos_fix.a"

# Windows static build using go-duckdb's bundled libduckdb.a
# GCC 15 (MinGW-w64) changed mbstate_t from struct _Mbstatet to int, causing
# a C++ ABI mismatch for basic_streambuf::seekpos. The libseekpos_fix stub
# supplies the missing symbol seekpos(fpos<_Mbstatet>) so static linking
# succeeds without patching go-duckdb or rebuilding DuckDB.
build:
	PATH="$(MINGW_BIN):$$PATH" \
	CGO_ENABLED=1 \
	CGO_LDFLAGS="-L$(LIBS_DIR) -lseekpos_fix" \
	go build \
	         -ldflags="-s -w" \
	         -o $(BINARY) ./cmd/flashmonitor

test:
	PATH="$(MINGW_BIN):$$PATH" \
	CGO_ENABLED=1 \
	CGO_LDFLAGS="-L$(LIBS_DIR) -lseekpos_fix" \
	go test ./... -coverprofile=cov.out
	go tool cover -html=cov.out -o coverage.html

clean:
	rm -f $(BINARY) cov.out coverage.html
