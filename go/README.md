# Invoice Extractor — Go

Go-native build of the local invoice → JSON service (parallel to `../python`).
Single binary, `net/http`, native Ollama Go client. Same `/extract` contract and
acceptance gates as the Python app — benchmarks are comparable across both.

See the [root README](../README.md) for the shared constraints and verified model tags.

## PDF backend decision: **poppler shell-out**

Chosen over `go-fitz` (CGo → MuPDF) to keep the build **pure-Go and trivially
cross-compilable** — no C toolchain or MuPDF dev headers needed. Cost is a runtime
dependency on `pdftotext`/`pdftoppm` (installed by the shared `scripts/setup_env.sh`).
The `pdftext.Backend` interface lets go-fitz be swapped in later (`PDF_BACKEND=gofitz`)
without touching callers.

## Layout
```
cmd/server       HTTP entrypoint (/health, /extract)
cmd/bench        benchmark harness (per-model, per-field scorecard)
cmd/visioncheck  vision sanity check (two-image discriminator)
internal/
  schema         RawInvoice (wire, all strings) + Invoice (canonical, typed)
  postprocess    Raw -> canonical: locale numbers, dates, total reconciliation (TDD)
  ollama         wraps github.com/ollama/ollama/api (text + vision chat)
  pdftext        poppler text-layer detect/extract (behind Backend interface)
  rasterize      pdftoppm -> PNG; image normalize (x/image)
  pipeline       text vs vision routing + fallback
  scoring        field-level accuracy (TDD)
  httpapi        handlers
  config         env-loaded config
testdata/        fixtures + labels (real ones gitignored; synthetic kept)
```

## The two-struct pattern (why)
Go's `encoding/json` is strict, not coercive. The model emits `"1.250.000,00"` /
`"21 Juni 2026"`; unmarshalling those straight into `decimal.Decimal`/`time.Time`
fails and loses the whole response. So `RawInvoice` is all `*string` (lenient wire),
and `postprocess.Normalize` coerces it into the typed, validated `Invoice`. Money is
always `shopspring/decimal`, never `float64`.

## Run / test / bench
```bash
cp .env.example .env
go build ./...                 # builds all packages + cmds
go test ./...                  # postprocess + scoring (no GPU needed)
go run ./cmd/server            # http://127.0.0.1:8000  -> GET /health
go run ./cmd/visioncheck       # vision check (needs Ollama + a pulled vision model)
go run ./cmd/bench             # benchmark decision tool (needs models + fixtures)

# end-to-end (needs Ollama running + models pulled + poppler installed):
curl -s -F file=@testdata/fixtures/synthetic_invoice.pdf localhost:8000/extract | jq
```

Fixtures are generated (shared with the Python app, for comparability) by:
```bash
python ../python/scripts/make_synthetic_fixture.py    # writes into go/testdata too
```

## Status (no GPU verified here)
- `go build ./...` / `go vet ./...` clean; `go test ./...` green (postprocess, scoring).
- `cmd/server` starts and `GET /health` → 200 (health-check gate).
- GPU-gated gates (PDF backend, routing, vision, extraction, benchmark) need Ollama + poppler installed and models pulled.
