package mlexport

// buildPyScript is the contents of scripts/build.py shipped in every package.
// It is dataset-agnostic: all parameters (mode, columns, split rule, file
// list) come from manifest.json and splits/split_v1.json at run time, and it
// mirrors the Go builder's conversion semantics exactly:
//   - every cell is a string; JSON numbers keep their original lexeme,
//     booleans become "true"/"false", null becomes "", nested values are
//     re-encoded as compact JSON with sorted keys;
//   - duplicate CSV headers are deduplicated with ".2", ".3", ... suffixes;
//   - sample IDs are "<raw file name>#<0-based row index>" (tabular) or the
//     raw file name (files mode);
//   - split assignment uses the explicit ID lists when the split file
//     includes them, otherwise frac = uint64(sha256("<seed>:<id>")[:8]) / 2^64
//     with the recorded ratios.
const buildPyScript = `#!/usr/bin/env python3
"""Rebuild processed/ from raw/ — deterministic, seeded.

Reads manifest.json and splits/split_v1.json from the package root and
regenerates the processed train/val/test outputs. Requires pyarrow for
tabular packages: pip install pyarrow
"""

import argparse
import csv
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_json(path):
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


def hash_split(sample_id, seed, ratios):
    digest = hashlib.sha256(f"{seed}:{sample_id}".encode("utf-8")).digest()
    frac = int.from_bytes(digest[:8], "big") / 2**64
    if frac < ratios["train"]:
        return "train"
    if frac < ratios["train"] + ratios["val"]:
        return "val"
    return "test"


def make_assigner(manifest, split_spec):
    """Return fn(sample_id, file_role) -> 'train'|'val'|'test'."""
    if split_spec.get("ids_included"):
        table = {}
        for name in ("train", "val", "test"):
            for sid in split_spec.get(name) or []:
                table[sid] = name
        return lambda sid, role: table[sid]
    if split_spec["method"] == "provided":
        role_map = {"train-split": "train", "validation-split": "val", "test-split": "test"}
        return lambda sid, role: role_map.get(role, "train")
    seed, ratios = split_spec["seed"], split_spec["ratios"]
    return lambda sid, role: hash_split(sid, seed, ratios)


def dedupe_columns(raw):
    used, out = set(), []
    for i, name in enumerate(raw):
        name = name.strip() or f"column_{i + 1}"
        candidate, n = name, 2
        while candidate in used:
            candidate = f"{name}.{n}"
            n += 1
        used.add(candidate)
        out.append(candidate)
    return out


class RawNum(str):
    """A JSON number kept as its original lexeme (so 1.50 stays '1.50')."""


def encode_json(value):
    """Compact JSON with sorted keys and raw number lexemes — byte-for-byte
    identical to how the DMS builder re-encodes nested values."""
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, RawNum):
        return str(value)
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=False)
    if isinstance(value, list):
        return "[" + ",".join(encode_json(v) for v in value) + "]"
    if isinstance(value, dict):
        return "{" + ",".join(
            json.dumps(k, ensure_ascii=False) + ":" + encode_json(v)
            for k, v in sorted(value.items())
        ) + "}"
    raise TypeError(f"unexpected JSON value type: {type(value)}")


def cell(value):
    if value is None:
        return ""
    if isinstance(value, str):  # includes RawNum lexemes
        return value
    if value is True:
        return "true"
    if value is False:
        return "false"
    return encode_json(value)


def iter_rows(path, detected_type):
    """Yield dict rows from one raw file, mirroring the DMS converter."""
    if detected_type in ("csv", "tsv"):
        delim = "\t" if detected_type == "tsv" else ","
        with open(path, "r", encoding="utf-8", newline="") as fh:
            reader = csv.reader(fh, delimiter=delim)
            header = next(reader, None)
            if header is None:
                return
            columns = dedupe_columns(header)
            for record in reader:
                yield {
                    name: (record[i] if i < len(record) else "")
                    for i, name in enumerate(columns)
                }
    elif detected_type == "jsonl":
        decoder = json.JSONDecoder(parse_int=RawNum, parse_float=RawNum)
        with open(path, "r", encoding="utf-8") as fh:
            buf = fh.read()
        pos, size = 0, len(buf)
        while True:
            while pos < size and buf[pos].isspace():
                pos += 1
            if pos >= size:
                break
            value, pos = decoder.raw_decode(buf, pos)
            if isinstance(value, dict):
                yield {k: cell(v) for k, v in value.items()}
            else:
                yield {"value": cell(value)}
    else:
        raise SystemExit(f"unsupported tabular type: {detected_type}")


def build_tabular(manifest, assigner, out_dir):
    import pyarrow as pa
    import pyarrow.parquet as pq

    columns = [c["name"] for c in manifest["schema"]["columns"]]
    id_column = manifest["schema"]["id_column"]
    buckets = {s: {c: [] for c in columns} for s in ("train", "val", "test")}

    for f in manifest["files"]:
        if f.get("role") not in ("data", "instruction-data", "train-split", "validation-split", "test-split"):
            continue
        if f.get("detected_type") not in ("csv", "tsv", "jsonl"):
            continue
        for idx, row in enumerate(iter_rows(ROOT / f["path"], f["detected_type"])):
            sid = f"{f['name']}#{idx}"
            bucket = buckets[assigner(sid, f.get("role"))]
            for c in columns:
                bucket[c].append(sid if c == id_column else row.get(c, ""))

    schema = pa.schema([(c, pa.string()) for c in columns])
    for split, data in buckets.items():
        table = pa.table({c: pa.array(data[c], type=pa.string()) for c in columns}, schema=schema)
        pq.write_table(table, out_dir / f"{split}.parquet", compression="snappy")
        print(f"wrote {split}.parquet ({table.num_rows} rows)")


def build_files(manifest, assigner, out_dir):
    buckets = {"train": [], "val": [], "test": []}
    for f in manifest["files"]:
        if f.get("role") not in ("data", "instruction-data", "train-split", "validation-split", "test-split"):
            continue
        sid = f["name"]
        buckets[assigner(sid, f.get("role"))].append(
            {"id": sid, "path": f["path"], "size_bytes": f["size_bytes"], "type": f.get("detected_type", "")}
        )
    for split, samples in buckets.items():
        with open(out_dir / f"{split}.jsonl", "w", encoding="utf-8") as fh:
            for s in samples:
                fh.write(json.dumps(s, ensure_ascii=False) + "\n")
        print(f"wrote {split}.jsonl ({len(samples)} samples)")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", default=str(ROOT / "processed"), help="output directory (default: processed/)")
    args = parser.parse_args()

    manifest = load_json(ROOT / "manifest.json")
    split_spec = load_json(ROOT / "splits" / manifest["split"]["file"].split("/")[-1])
    assigner = make_assigner(manifest, split_spec)

    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    if manifest["schema"]["mode"] == "tabular":
        build_tabular(manifest, assigner, out_dir)
    else:
        build_files(manifest, assigner, out_dir)
    print("done — split counts should match splits/" + manifest["split"]["file"].split("/")[-1])


if __name__ == "__main__":
    sys.exit(main())
`
