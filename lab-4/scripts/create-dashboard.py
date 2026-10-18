import json
from pathlib import Path

panels = []
datasource = {"type": "prometheus", "uid": "prometheus"}

def panel(title, expr, unit="short", legend="{{pod}}", maximum=None, sla=False):
    index = len(panels)
    defaults = {
        "unit": unit,
        "min": 0,
        "thresholds": {
            "mode": "absolute",
            "steps": [{"color": "green", "value": None}],
        },
    }
    if maximum is not None:
        defaults["max"] = maximum
    if sla:
        defaults["thresholds"]["steps"].append(
            {"color": "red", "value": 0.3}
        )
        defaults["custom"] = {"thresholdsStyle": {"mode": "line"}}

    panels.append({
        "id": index + 1,
        "type": "timeseries",
        "title": title,
        "datasource": datasource,
        "gridPos": {
            "x": (index % 2) * 12,
            "y": (index // 2) * 8,
            "w": 12,
            "h": 8,
        },
        "targets": [{
            "refId": "A",
            "datasource": datasource,
            "expr": expr,
            "legendFormat": legend,
        }],
        "fieldConfig": {"defaults": defaults, "overrides": []},
        "options": {
            "legend": {"displayMode": "list", "placement": "bottom"},
            "tooltip": {"mode": "multi"},
        },
    })

# Переменные namespace/release позволяют использовать другой Helm-релиз.
api = 'namespace="$namespace",service="$release-api"'
worker = 'namespace="$namespace",service="$release-worker"'
containers = (
    'namespace="$namespace",pod=~"$release-(api|worker|postgres|batch)-.*",'
    'container!="",container!="POD",'
    'job="kubelet",metrics_path="/metrics/cadvisor"'
)

panel(
    "POST /order — HTTP p95; порог 300 мс",
    'histogram_quantile(0.95, sum by (le) '
    f'(rate(http_request_duration_seconds_bucket{{{api},path="/order"}}[2m])))',
    "s", "HTTP p95", sla=True,
)
panel(
    "POST /order — доля HTTP 5xx",
    f'(sum(rate(http_requests_total{{{api},path="/order",status=~"5.."}}[2m])) '
    'or vector(0)) / clamp_min('
    f'sum(rate(http_requests_total{{{api},path="/order"}}[2m])), 0.000001)',
    "percentunit", "HTTP 5xx", maximum=1,
)
panel(
    "POST /order — запросы в секунду",
    f'sum(rate(http_requests_total{{{api},path="/order"}}[2m]))',
    "reqps", "Создание заказа",
)
panel(
    "Worker — обработанные заказы в секунду",
    f'sum(rate(worker_orders_processed_total{{{worker}}}[2m]))',
    "ops", "Обработано",
)
panel(
    "CPU — потребление по контейнерам",
    f'sum by (pod, container) (rate(container_cpu_usage_seconds_total{{{containers}}}[2m]))',
    "cores", "{{pod}} / {{container}}",
)
panel(
    "CPU — доля периодов с throttling",
    f'100 * sum by (pod, container) (rate(container_cpu_cfs_throttled_periods_total{{{containers}}}[2m]))'
    f' / sum by (pod, container) (rate(container_cpu_cfs_periods_total{{{containers}}}[2m]))',
    "percent", "{{pod}} / {{container}}", maximum=100,
)
panel(
    "Память — working set",
    f'max by (pod, container) (container_memory_working_set_bytes{{{containers}}})',
    "bytes", "{{pod}} / {{container}}",
)
panel(
    "Batch — число подов по состояниям",
    'sum by (phase) (max by (pod, phase) '
    '(kube_pod_status_phase{namespace="$namespace",'
    'pod=~"$release-batch-.*",phase=~"Pending|Running|Failed"}))',
    "short", "{{phase}}",
)
panel(
    "Готовые реплики Deployment",
    'max by (deployment) (kube_deployment_status_replicas_available{'
    'namespace="$namespace",deployment=~"$release-(api|worker|batch)"})',
    "short", "{{deployment}}",
)
panel(
    "Алерты Shop — pending/firing",
    'ALERTS{namespace="$namespace",alertname=~"Shop.*"}',
    "short", "{{alertname}} / {{alertstate}}", maximum=1,
)

dashboard = {
    "id": None,
    "uid": "shop-lab4",
    "title": "Shop — лабораторная 4: SLA и ресурсы",
    "tags": ["shop", "lab-4"],
    "schemaVersion": 39,
    "version": 1,
    "timezone": "browser",
    "refresh": "15s",
    "time": {"from": "now-30m", "to": "now"},
    "templating": {"list": [
        {
            "name": "namespace", "type": "textbox", "label": "Namespace",
            "query": "shop", "current": {"text": "shop", "value": "shop"},
        },
        {
            "name": "release", "type": "textbox", "label": "Release",
            "query": "shop", "current": {"text": "shop", "value": "shop"},
        },
    ]},
    "panels": panels,
}

path = Path("shop-chart/dashboards/shop-lab4.json")
path.write_text(json.dumps(dashboard, ensure_ascii=False, indent=2) + "\n")
print("Создан:", path)
