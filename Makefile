PREFIX  ?= /usr/local
BINDIR  ?= $(PREFIX)/bin

PROFDIR ?= $(shell git rev-parse --short HEAD)

# change to your device to test with
dev/air:
	air -build.args_bin "--serial ${SERIAL}"

templates:
	templ generate

css:
	tailwindcss -i ./internal/web/static/css/input.css -o ./internal/web/static/css/dist/style.css

dev: templates css
	go run ./cmd/noreza/main.go --wait --serial "${SERIAL}"

profile: templates css
	@rm -rf ./$(PROFDIR)/*.prof
	@mkdir -p ./$(PROFDIR)
	go run ./cmd/noreza/main.go --serial "${SERIAL}" --cpuprofile ./$(PROFDIR)/cpu.prof --memprofile ./$(PROFDIR)/mem.prof --latprofile ./$(PROFDIR)/lat.prof

compare:
	@echo "=== LATENCY ==="
	@head -9 $(BEFORE)/lat.prof $(AFTER)/lat.prof
	@echo
	@echo "=== CPU ==="
	@echo "Before:"
	@go tool pprof -top -nodecount=10 $(BEFORE)/cpu.prof 2>&1 | head -20
	@echo
	@echo "After:"
	@go tool pprof -top -nodecount=10 $(AFTER)/cpu.prof 2>&1 | head -20
	@echo
	@echo "=== MEMORY ==="
	@echo "Before:"
	@go tool pprof -top -nodecount=10 $(BEFORE)/mem.prof 2>&1 | head -20
	@echo
	@echo "After:"
	@go tool pprof -top -nodecount=10 $(AFTER)/mem.prof 2>&1 | head -20

build:
	go build ./cmd/noreza

install: noreza
	@echo "Installing noreza to $(BINDIR)"
	install -d $(BINDIR)
	install -m 0755 ./noreza $(BINDIR)

.PHONY: dev dev/air profile templates css compare install build

