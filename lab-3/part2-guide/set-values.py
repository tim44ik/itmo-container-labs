from pathlib import Path
import re, sys
options = {
    "api.replicas": (r"(?m)^  replicas:.*$", "  replicas: {}", lambda v: v.isdigit() and int(v)>0),
    "api.image.tag": (r"(?m)^    tag:.*$", "    tag: {}", lambda v: bool(re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]*",v))),
    "api.healthFail": (r"(?m)^  healthFail:.*$", "  healthFail: {}", lambda v: v in ("true","false"))
}
if len(sys.argv)!=3 or sys.argv[1] not in options:
    raise SystemExit("Usage: set-values.py api.replicas|api.image.tag|api.healthFail VALUE")
key,value=sys.argv[1:]
pattern,replacement,valid=options[key]
if not valid(value): raise SystemExit("Invalid value")
path=Path("shop-chart/values.yaml")
text=path.read_text()
match=re.search(r"(?ms)^api:\n.*?(?=^[A-Za-z][A-Za-z0-9_-]*:|\Z)", text)
if not match: raise SystemExit("api section not found")
section,count=re.subn(pattern,replacement.format(value),match.group())
if count!=1: raise SystemExit("Expected exactly one matching key")
path.write_text(text[:match.start()]+section+text[match.end():])
print(f"{key} = {value} saved in {path}")
