# mlexport

Builds the downloadable **ML package** for a dataset: a structured, ML-ready
zip served by `GET /api/datasets/:id/export`.

## Package layout

```
<dataset-slug>/
├── README.md          # datasheet: provenance, contents, schema, labels, biases
├── manifest.json      # version, sha256 checksums, counts, schema, label definitions
├── raw/               # every file exactly as uploaded — never edited
├── processed/         # tabular: train/val/test.parquet (all values as strings,
│                      # plus a sample_id column); otherwise train/val/test.jsonl
│                      # whose paths reference raw/ (no duplication)
├── splits/split_v1.json   # the split IS data: method, seed, ratios, sample IDs
└── scripts/build.py   # raw → processed, deterministic, seeded (needs pyarrow)
```

## How it works

- **`Builder`** (`builder.go`) is database-free: it takes a `PackageSpec` and
  produces the zip, streaming raw files straight from storage (never copied).
  This is what the unit tests exercise.
- **`Service`** (`service.go`) loads the spec from PostgreSQL, runs the
  builder, and tracks state in `datasets.export_status` /
  `export_progress` (`none | building | ready | error`, progress 0–1).
  Raw-file SHA-256 digests are persisted to `files.sha256`.
- The AI pipeline flips `export_status='building'` in the same UPDATE that
  sets `status='ready'`, then calls `Service.BuildNow` — so the dataset is
  searchable immediately while the package builds, and clients never observe
  `ready` + `none` on new datasets.
- Pre-feature datasets are backfilled lazily: the dataset detail handler
  calls `Service.StartIfNeeded` on first view (guarded by an atomic UPDATE,
  so only one builder ever runs per dataset).
- On reprocess the pipeline calls `Service.Reset` first, so a stale package
  is never served after files change.

## Splits

If the uploader shipped explicit train/val/test files (profiler roles
`train-split` / `validation-split` / `test-split`, at least two present),
that assignment is honored (`method: "provided"`). Otherwise samples are
assigned by a deterministic seeded hash (`method: "hash"`, seed 42,
80/10/10): `frac = uint64(sha256("<seed>:<id>")[0:8]) / 2^64`.

Sample IDs are `<raw file name>#<0-based row index>` for tabular data and
the raw file name for file-based datasets. Explicit ID lists are written to
`splits/split_v1.json` up to 250 000 samples; beyond that only the rule and
counts are recorded (the assignment stays fully deterministic).

## Determinism

`scripts/build.py` mirrors the Go conversion exactly (string cells, number
lexemes preserved, nested JSON re-encoded compactly with sorted keys,
identical hash rule). `TestBuildPyReproducesProcessed` proves it: it builds
a package, runs the shipped script with real Python/pyarrow, and diffs the
rebuilt parquet against the Go output. That test needs
`MLEXPORT_PYTHON=/path/to/python-with-pyarrow` and is skipped otherwise.
