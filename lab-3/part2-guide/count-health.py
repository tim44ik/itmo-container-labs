import sys
from pathlib import Path
lines=Path(sys.argv[1]).read_text().splitlines()
ok=sum(line.endswith(" OK") for line in lines)
failed=sum(line.endswith(" FAIL") for line in lines)
print(f"OK={ok} FAIL={failed}")
if ok==0 or failed:
    raise SystemExit("Measurement did not confirm an error-free interval.")
