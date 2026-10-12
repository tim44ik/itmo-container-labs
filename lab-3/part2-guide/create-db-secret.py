import json, secrets, subprocess
base = ["kubectl", "--context", "kind-labs", "-n", "shop"]
existing = subprocess.run(base + ["get", "secret", "shop-db", "--ignore-not-found", "-o", "name"], capture_output=True, text=True, check=True)
if existing.stdout.strip():
    raise SystemExit("Secret shop-db already exists. Do not overwrite it; check its purpose before continuing.")
password = secrets.token_hex(24)
doc = {
    "apiVersion":"v1", "kind":"Secret",
    "metadata":{"name":"shop-db", "namespace":"shop"},
    "type":"Opaque", "stringData": {
        "username":"shop", "password":password, "dbname":"shop",
        "uri":f"postgresql://shop:{password}@shop-postgres-temp.shop.svc.cluster.local:5432/shop?sslmode=disable&connect_timeout=3"
    }
}
subprocess.run(base + ["create", "-f", "-"], input=json.dumps(doc), text=True, check=True)
print("Secret created; password was not written to a local file.")
