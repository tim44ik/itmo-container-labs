# Отчёт по лабораторной работе 5

## Zero-trust сеть для приложения shop

**Среда:** Ubuntu, Docker, kind `lab5`, Kubernetes v1.32.2, Cilium v1.17.4.  
**Дата выполнения:** 25 октября 2026 г.  
**Цель:** развернуть приложение в Kubernetes, исследовать работу Service, ограничить
связность с помощью NetworkPolicy, обнаружить неисправность по сетевым потокам и
проверить метрики в Prometheus.

## Часть 1. Развёртывание стенда

Для экспериментов создан отдельный кластер kind `lab5` с control-plane и двумя
worker-узлами. Все узлы перешли в состояние `Ready`:

| Узел | Роль | Kubernetes | IP | Состояние |
| --- | --- | --- | --- | --- |
| `lab5-control-plane` | control-plane | v1.32.2 | `172.18.0.2` | Ready |
| `lab5-worker` | worker | v1.32.2 | `172.18.0.3` | Ready |
| `lab5-worker2` | worker | v1.32.2 | `172.18.0.4` | Ready |

В кластере установлен Cilium v1.17.4; `cilium status` сообщил `Cilium: OK`,
`Operator: OK`, `Hubble Relay: OK`. Запущены три агента Cilium и три экземпляра
kube-proxy. Сервисы установлены Helm:

| Release | Namespace | Версия | Статус |
| --- | --- | --- | --- |
| `cilium` | `kube-system` | 1.17.4 | deployed |
| `cnpg` | `cnpg-system` | 0.29.1 | deployed |
| `jaeger` | `monitoring` | 4.16.1 | deployed |
| `kube-prometheus` | `monitoring` | 92.13.1 | deployed |
| `shop` | `shop` | 0.1.0 | deployed |

PostgreSQL CloudNativePG (`shop-postgres`) работает в состоянии `Ready`, с одним
экземпляром. API опубликован Service типа NodePort на `30080`; kind пробрасывает
его на `127.0.0.1:18080`.

<details>
<summary>Команды проверки кластера и релизов</summary>

Развёртывание выполнялось в отдельном кластере, с Cilium вместо встроенного CNI:

```bash
kind create cluster --config infra/kind-lab5.yaml --wait 5m
cilium install --version 1.17.4 --namespace kube-system \
  --helm-values infra/cilium-lab5-values.yaml
cilium status --wait
```

После установки операторов и приложения проверялись состояние узлов и релизов:

```bash
kubectl --context kind-lab5 get nodes -o wide
cilium status --context kind-lab5
kubectl --context kind-lab5 -n kube-system get ds kube-proxy
helm list --kube-context kind-lab5 -A
kubectl --context kind-lab5 -n shop get pods,svc,endpointslices -o wide
kubectl --context kind-lab5 -n shop get cluster shop-postgres -o wide
kubectl --context kind-lab5 -n shop wait \
  --for=condition=Ready cluster/shop-postgres --timeout=30s
curl -i --connect-timeout 3 --max-time 6 http://127.0.0.1:18080/health
curl -i --connect-timeout 3 --max-time 6 http://127.0.0.1:18080/ready
```

</details>

## Часть 2. Исследование исходной сети и Service

Проверены Pod-связность между worker-узлами, DNS и Kubernetes Service. Два
диагностических Pod’а получили адреса `10.244.2.164` и `10.244.1.238`.
Имя `shop-api` разрешалось в ClusterIP `10.96.235.97`; ping между Pod’ами на
разных узлах прошёл без потерь.

Устойчивость NodePort проверена параллельно с удалением готовых Pod’ов API и
worker. Зафиксированы результаты:

- удалено 20 готовых Pod’ов в ходе chaos-теста;
- 150 запросов завершились HTTP 200;
- ошибок запросов: 0;
- средняя задержка: `0.011535` с;
- максимальная задержка: `0.061213` с.

Данные и график этого прогона сохранены в
[`evidence/load-test-live.csv`](evidence/load-test-live.csv) и
[`evidence/load-test-live.svg`](evidence/load-test-live.svg). В репозитории также
есть прежний прогон: 167 успешных ответов HTTP 200, см.
[`evidence/load-test.csv`](evidence/load-test.csv) и
[`evidence/load-test.svg`](evidence/load-test.svg).

<details>
<summary>Команды проверки сети, нагрузки и chaos-теста</summary>

Диагностические Pod’ы запускались на разных worker-узлах:

```bash
kubectl --context kind-lab5 -n shop run net-debug-a \
  --image=nicolaka/netshoot:v0.13 --restart=Never \
  --overrides='{"apiVersion":"v1","spec":{"nodeName":"lab5-worker"}}' -- sleep 3600
kubectl --context kind-lab5 -n shop run net-debug-b \
  --image=nicolaka/netshoot:v0.13 --restart=Never \
  --overrides='{"apiVersion":"v1","spec":{"nodeName":"lab5-worker2"}}' -- sleep 3600
kubectl --context kind-lab5 -n shop wait \
  --for=condition=Ready pod/net-debug-a pod/net-debug-b --timeout=2m
kubectl --context kind-lab5 -n shop exec net-debug-a -- getent hosts shop-api
NET_DEBUG_B_IP=$(kubectl --context kind-lab5 -n shop get pod net-debug-b \
  -o jsonpath='{.status.podIP}')
kubectl --context kind-lab5 -n shop exec net-debug-a -- ping -c 3 "$NET_DEBUG_B_IP"
```

Live-нагрузка и удаление готовых Pod’ов запускались так:

```bash
DURATION_SECONDS=180 OUTPUT=lab-5/evidence/load-test-live.csv \
  bash scripts/lab5/load-test.sh
```

Эту команду и chaos-тест запускали одновременно, в разных терминалах:

```bash
CONFIRM_CHAOS=YES MAX_DELETIONS=20 INTERVAL_SECONDS=5 \
  bash scripts/lab5/chaos-pods.sh
```

После завершения обоих процессов график строится отдельно:

```bash
python3 scripts/lab5/plot-load-test.py \
  lab-5/evidence/load-test-live.csv lab-5/evidence/load-test-live.svg
```

</details>

## Часть 3. Zero-trust NetworkPolicy и проверка матрицы

В namespace `shop` применены default-deny и разрешающие NetworkPolicy для
необходимых путей: клиент → API, API/worker → PostgreSQL, DNS, мониторинг и
телеметрия. Доверенный `lab5-client` успешно обращался к API, в том числе через
Service DNS.

Сетевые проверки от недоверенного Pod дали ожидаемый результат:

| Источник | Назначение | Результат |
| --- | --- | --- |
| `lab5-outsider` | PostgreSQL `:5432` | подключение запрещено |
| `lab5-outsider` | API Service `:8080` | подключение запрещено |
| `lab5-outsider` | API Pod `:8080` | подключение запрещено |
| `lab5-client` | API `/health` и `/ready` | HTTP 200 |

Отказы для outsider подтверждены Hubble, например:

```text
shop/lab5-outsider <> shop/shop-postgres-1:5432
Policy denied DROPPED (TCP Flags: SYN)

shop/lab5-outsider <> shop/shop-api-5bbcfdb8c9-mrgg7:8080
Policy denied DROPPED (TCP Flags: SYN)
```

При первоначальной проверке внешний NodePort не отвечал. Hubble показал, что
фактический источник — `10.244.0.37 (world)`, а не ожидаемый Docker bridge CIDR:

```text
10.244.0.37 (world) <> shop/shop-api:8080
INGRESS DENIED
Policy denied DROPPED (TCP Flags: SYN)
```

Для исправления в Helm-чарт добавлена CiliumNetworkPolicy: она разрешает entity
`world` вход только на TCP:8080 API. После применения разрешённый поток стал
виден как:

```text
10.244.0.37 (world) -> shop/shop-api:8080
policy-verdict:L3-L4 INGRESS ALLOWED (TCP Flags: SYN)
```

Это правило проверено в kind-профиле, где host-порт привязан к loopback
`127.0.0.1`; оно не должно без адаптации переноситься в публичный кластер.

Также Hubble выявил попытки PostgreSQL обратиться к Kubernetes API на TCP:6443.
Для CloudNativePG добавлено отдельное правило Cilium, разрешающее только entity
`kube-apiserver` и порт `6443`. Это устраняет блокировку служебного трафика CNPG,
не открывая произвольный egress.

<details>
<summary>Команды проверки NetworkPolicy и потоков Hubble</summary>

Для Hubble запустить port-forward в отдельном терминале:

```bash
cilium hubble port-forward --context kind-lab5 --port-forward 4245
```

В другом терминале выполнялись клиентские проверки и наблюдение потоков:

```bash
kubectl --context kind-lab5 -n shop exec lab5-client -- \
  curl -fsS --max-time 5 http://shop-api:8080/health

POSTGRES_POD_IP=$(kubectl --context kind-lab5 -n shop get pods \
  -l cnpg.io/cluster=shop-postgres -o jsonpath='{.items[0].status.podIP}')
API_CLUSTER_IP=$(kubectl --context kind-lab5 -n shop get svc shop-api \
  -o jsonpath='{.spec.clusterIP}')
kubectl --context kind-lab5 -n shop exec lab5-outsider -- \
  nc -z -w 2 "$POSTGRES_POD_IP" 5432
kubectl --context kind-lab5 -n shop exec lab5-outsider -- \
  nc -z -w 2 "$API_CLUSTER_IP" 8080

hubble observe --server localhost:4245 --from-ip 10.244.1.214 \
  --to-port 5432 --since 3m --verdict DROPPED
hubble observe --server localhost:4245 --from-ip 10.244.1.214 \
  --to-port 8080 --since 3m --verdict DROPPED
hubble observe --server localhost:4245 --from-ip 10.244.0.37 \
  --to-port 8080 --since 3m
hubble observe --server localhost:4245 --from-ip 10.244.2.106 \
  --to-port 6443 --since 30s
kubectl --context kind-lab5 -n shop get networkpolicy,ciliumnetworkpolicy
```

</details>

## Часть 4. Диагностика скрытой поломки

Для fault injection был выбран вариант с отключением DNS в NetworkPolicy.
Симптом проявился в том, что `lab5-client` перестал разрешать имя `shop-api`, а
проверка API через Service DNS завершилась таймаутом. Hubble зафиксировал
запрещённые DNS-запросы:

```text
shop/lab5-client <> kube-system/coredns-668d6bf9bc-bqhz4:53
EGRESS DENIED
Policy denied DROPPED (UDP)
```

После диагностики просмотр fault overlay подтвердил отключённое DNS-разрешение;
файл сохранён как [`evidence/fault-values.yaml`](evidence/fault-values.yaml).
Затем применён базовый overlay Helm. Повторные проверки подтвердили восстановление:

- `shop-api` снова разрешается в `10.96.235.97`;
- API `/health` и `/ready` через доверенный клиент отвечают `ok`;
- внешний NodePort `/ready` также отвечает `ok`.

<details>
<summary>Команды внедрения, диагностики и восстановления</summary>

```bash
# Скрипт случайно выбирает и применяет неисправность в кластере.
CONFIRM_FAULT=YES bash scripts/lab5/inject-fault.sh

# Сначала зафиксировать симптом и сетевые события:
kubectl --context kind-lab5 -n shop exec lab5-client -- \
  curl -fsS --max-time 4 http://shop-api:8080/health
hubble observe --server localhost:4245 --from-ip 10.244.2.196 \
  --to-port 53 --since 2m

# После фиксации симптома изучить выбранную fault-конфигурацию:
cat lab-5/evidence/fault-values.yaml

# Восстановить базовые значения и повторить проверки:
helm upgrade --install shop ./shop-chart --kube-context kind-lab5 \
  --namespace shop -f shop-chart/values-lab5.yaml --wait --timeout 10m
kubectl --context kind-lab5 -n shop rollout status deployment/shop-api
kubectl --context kind-lab5 -n shop exec lab5-client -- getent hosts shop-api
kubectl --context kind-lab5 -n shop exec lab5-client -- \
  curl -fsS --max-time 5 http://shop-api:8080/ready
curl -fsS --max-time 5 http://127.0.0.1:18080/ready
```

</details>

## Часть 5. Метрики приложения и алерты

Prometheus targets API и worker доступны; все пять scrape targets приложения
имели состояние `up`. Число обработанных заказов по метрике worker достигло 2
на момент проверки.

Для проверки HTTP-алерта были отправлены запросы на `/fail`, возвращающий
HTTP 500. `ShopApiHighErrorRatio` прошёл состояния `pending` и `firing`, когда
доля ошибок превысила 5%, а после завершения тестового трафика вернулся в
`inactive`.

Отдельно проверен DNS через запрос к несуществующему домену: CoreDNS вернул
ожидаемый `NXDOMAIN`, и счётчик `coredns_dns_responses_total` с меткой `rcode`
увеличился. Это ожидаемый ответ DNS, не ошибка `SERVFAIL`; поэтому
`ShopDNSFailures` не срабатывал. Запросы Prometheus и дашборд исправлены так,
чтобы использовать реально собираемую метрику CoreDNS, а не отсутствующую в
этом Prometheus серию `hubble_dns_responses_total`.

Состояния пользовательских алертов после последней проверки:

| Алерт | Состояние | Комментарий |
| --- | --- | --- |
| `ShopTargetDown` | inactive | targets API/worker доступны |
| `ShopApiHighErrorRatio` | inactive | тест `/fail` завершён |
| `ShopNetworkPolicyDrops` | firing | повторно проверены запрещённые соединения outsider |
| `ShopDNSFailures` | inactive | NXDOMAIN не является SERVFAIL |
| `ShopWorkerErrors` | inactive | при последней проверке новых ошибок не было |

`ShopNetworkPolicyDrops` находился в `firing` после тестирования outsider.
Это ожидаемо: правило агрегирует drops за временное окно. После прекращения
запросов следует дождаться истечения окна и повторно проверить статус алерта.

<details>
<summary>Команды проверки targets, метрик и алертов</summary>

Prometheus API был доступен через port-forward:

```bash
kubectl --context kind-lab5 -n monitoring port-forward \
  svc/kube-prometheus-kube-prome-prometheus 9090:9090
```

```bash
# Создать HTTP 500 трафик для ShopApiHighErrorRatio:
for i in $(seq 1 30); do
  curl -sS -o /dev/null http://127.0.0.1:18080/fail
done

# NXDOMAIN: ожидаемый ответ, а не SERVFAIL:
kubectl --context kind-lab5 -n shop exec lab5-client -- \
  curl -sS --max-time 4 http://does-not-exist.invalid

# Проверить доступность targets:
curl -fsS 'http://127.0.0.1:9090/api/v1/targets?state=active'

# Получить состояние Shop alert rules:
curl -fsS 'http://127.0.0.1:9090/api/v1/rules?type=alert' |
  jq -r '.data.groups[].rules[]
    | select(.name | startswith("Shop"))
    | [.name, .state, (.alerts | length)] | @tsv'

# Примеры использованных PromQL выражений:
# sum(rate(http_requests_total{namespace="shop",service="shop-api"}[2m]))
# sum(increase(coredns_dns_responses_total{job="coredns"}[5m]))
```

</details>

## Часть 6. Метрики Cilium и Hubble

Включены Cilium/Hubble ServiceMonitor и сбор сетевых метрик. В Prometheus
подтверждены активные targets трёх Cilium agents, трёх Hubble metrics endpoints
и CoreDNS.

Проверены Hubble flow и drop-события. Для разрешённого NodePort-запроса зафиксирован
`INGRESS ALLOWED`; попытки outsider к API и PostgreSQL получили `DROPPED`.
Для DNS метрики используются `coredns_dns_responses_total`, включая метку `rcode`.

<details>
<summary>Команды проверки сетевых метрик и потоков</summary>

```bash
# Запустить в отдельном терминале:
cilium hubble port-forward --context kind-lab5 --port-forward 4245
```

В другом терминале выполнять наблюдение и запросы к Prometheus:

```bash
# Наблюдать drop и разрешённые потоки:
hubble observe --server localhost:4245 --verdict DROPPED --since 5m
hubble observe --server localhost:4245 --namespace shop --since 5m
hubble observe --server localhost:4245 --from-ip 10.244.0.37 \
  --to-port 8080 --since 3m

# Проверить метрики в Prometheus API:
curl -fsS -G 'http://127.0.0.1:9090/api/v1/query' \
  --data-urlencode \
  'query=sum(increase(hubble_flows_processed_total{verdict="DROPPED",source_namespace="shop"}[5m]))'
curl -fsS -G 'http://127.0.0.1:9090/api/v1/query' \
  --data-urlencode \
  'query=sum(increase(coredns_dns_responses_total{job="coredns"}[5m]))'
```

</details>

## Часть 7. Мониторинг сети и итоговая проверка

Prometheus, Grafana и Jaeger работают в namespace `monitoring`; Grafana Pod
состоял из трёх готовых контейнеров (`3/3`). Targets приложения, Cilium, Hubble
и CoreDNS доступны. Grafana dashboard обновлён: DNS-график использует
`coredns_dns_responses_total`, а сетевые панели показывают потоки, drops и TCP.

Визуальные скриншоты Grafana и Hubble во время этого прогона не сохранялись;
фактические состояния подтверждены запросами к API Prometheus и CLI Hubble.

После восстановления базовой конфигурации выполнен итоговый запрос к NodePort:

```text
GET /health  -> HTTP 200, ok
GET /ready   -> HTTP 200, ok
```

Внешний заказ прошёл полный путь через API, PostgreSQL и worker:

```text
POST /order -> HTTP 201
item: nodeport-after-policy-fix
status: processed
```

Все API и worker Pod’ы были `Running`, PostgreSQL — `Ready`, Cilium — `OK`.
Результат показывает, что при включённых политиках разрешённый путь приложения
сохранил работоспособность, а несанкционированные подключения были заблокированы
и обнаружены через Hubble/Prometheus.

<details>
<summary>Команды итоговой проверки и Grafana</summary>

```bash
curl -i --connect-timeout 3 --max-time 6 http://127.0.0.1:18080/health
curl -i --connect-timeout 3 --max-time 6 http://127.0.0.1:18080/ready
curl -i --connect-timeout 3 --max-time 6 \
  -H 'Content-Type: application/json' \
  -d '{"item":"nodeport-after-policy-fix","quantity":1}' \
  http://127.0.0.1:18080/order
curl -fsS http://127.0.0.1:18080/orders

# Grafana UI, в отдельном терминале:
kubectl --context kind-lab5 -n monitoring port-forward \
  svc/kube-prometheus-grafana 3000:80
```

</details>

## Артефакты

- Live-нагрузка: [`evidence/load-test-live.csv`](evidence/load-test-live.csv),
  [`evidence/load-test-live.svg`](evidence/load-test-live.svg).
- Предыдущая нагрузка: [`evidence/load-test.csv`](evidence/load-test.csv),
  [`evidence/load-test.svg`](evidence/load-test.svg).
- Параметры fault injection: [`evidence/fault-values.yaml`](evidence/fault-values.yaml).
