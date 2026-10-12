from pathlib import Path
path=Path("shop-chart/templates/api-deployment.yaml")
text=path.read_text()
if "lifecycle:" in text:
    raise SystemExit("lifecycle already exists. Inspect it before changing.")
needle="          resources:\n"
if text.count(needle)!=1: raise SystemExit("Unexpected template structure")
addition="""          lifecycle:
            preStop:
              sleep:
                seconds: 5
"""
path.write_text(text.replace(needle,addition+needle,1))
print("Added preStop sleep 5s to API template (Kubernetes 1.37 supports this action).")
