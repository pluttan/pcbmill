"""DRC of the test board as a check of the generator.

python3 testboard/drc_summary.py build/testboard/drc.json
The samples go under the rules on purpose (gaps from 0.15, thin tracks);
they are listed for reference only. A short between two nets or a net in
pieces is a fault of the generator and fails the build.
"""

import collections
import json
import sys

FAULTS = {"shorting_items", "tracks_crossing", "unconnected_items"}
SKIP = {"track_dangling"}  # the free ends of the comb fingers


def main():
    d = json.load(open(sys.argv[1], encoding="utf-8"))
    by = collections.Counter()
    bad = []
    for v in d.get("violations", []):
        if v["type"] in SKIP:
            continue
        by[v["type"]] += 1
        if v["type"] in FAULTS:
            bad.append(v["description"] + ": " + "; ".join(i.get("description", "") for i in v.get("items", [])))
    print("образцы ниже правил (так задумано):", ", ".join(f"{k} {n}" for k, n in sorted(by.items())) or "нет")
    un = d.get("unconnected_items", [])
    for u in un[:10]:
        bad.append("разрыв: " + "; ".join(i.get("description", "") for i in u.get("items", [])))
    if bad:
        for b in bad[:20]:
            print("ОШИБКА ГЕНЕРАТОРА:", b)
        sys.exit(1)
    print("замыканий и разрывов нет")


if __name__ == "__main__":
    main()
