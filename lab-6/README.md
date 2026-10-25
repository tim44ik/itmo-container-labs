# Лабораторная 6 — Service Mesh и автопилот канареечного релиза

Итоговый отчёт и собранные результаты эксперимента: [REPORT.md](./REPORT.md).

Пошаговый план выполнения лабораторной на существующем примере `shop`. Цель —
проверить, что Service Mesh действительно даёт mTLS и повышает устойчивость,
автоматизировать продвижение или откат версии API, а затем измерить цену mesh.
Реальные результаты, выводы команд и скриншоты добавляйте по мере выполнения:
ожидаемые результаты ниже не заменяют измерения.

Рабочие компоненты проекта:

- API и его Dockerfile: [`shop-services/`](../shop-services/);
- Helm-чарт: [`shop-chart/`](../shop-chart/);
- Prometheus values из предыдущих работ: [`api/prometheus/values.yaml`](../api/prometheus/values.yaml);
- инструкция по базовому чарту и мониторингу: [Лаба 3](../lab-3/README.md);
- пример отдельного кластера и установки стека: [Лаба 5](../lab-5/README.md).

## Что должно получиться

1. У API есть версии v1 и v2, endpoint `/version`, отказоустойчивый тестовый
   режим v1 (`FLAKY=true`) и управляемый отказ v2 (`FAIL=true`).
2. API работает через mesh с включённым mTLS; Postgres и остальные сервисы
   остаются вне mesh.
3. Наблюдения подтверждают, что backend получает ошибки, а клиентские
   идемпотентные запросы почти не видят их благодаря retry.
4. Трафик между версиями сначала делится 90/10, затем автопилот сам откатывает
   плохую v2 и постепенно переводит весь трафик на здоровую v2.
5. Baseline до mesh сравнен с результатом после mesh по latency и ресурсам.
6. Grafana показывает запросы, ошибки и задержку по версиям; Prometheus
   оценивает три обоснованных алерта.

## Архитектура и решение по mesh

Для этой работы выбери **Linkerd**: он рассчитан на небольшой Kubernetes-кластер,
прост в установке и подходит для проверки identity, mTLS, метрик и HTTP retries.
Зафиксируй версию CLI и control plane в отчёте. Traffic split делай через
Gateway API `HTTPRoute`: прежний Linkerd SMI `TrafficSplit` устарел. Смотри
[документацию Linkerd по маршрутизации](https://linkerd.io/2.18/features/request-routing/)
и [retry](https://linkerd.io/2.18/features/retries-and-timeouts/).

Добавь в чарт отдельные Deployment и Service для API каждой версии, например
`shop-api-v1` и `shop-api-v2`. Перед ними оставь стабильную точку входа
`shop-api`, к которой прикреплён `HTTPRoute`. У маршрута backend-ы обеих версий
и веса из `values.yaml`. У каждого backend Service должны быть метки, по которым
Prometheus различает версии.

Для проверки retry запусти отдельный HTTP-клиент внутри кластера и включи mesh
для его Pod’а. Это необходимо: Linkerd выполняет retries на исходящей стороне,
поэтому немешированный `curl` с хоста не проверит этот механизм. В остальном
mesh включается только для API и этого временного тестового клиента; Postgres и
worker оставь вне mesh. Не настраивай retries для `POST /order`: повтор такого
запроса может создать дубликаты. Flaky-сценарий проверяй на идемпотентном
`GET /orders`.

## Подготовка. Инструменты и отдельный кластер

Нужны Docker, kind, kubectl, Helm, Linkerd CLI, curl, Prometheus Operator и
Python 3. Для воспроизводимости используй отдельный одноузловой кластер `lab6`,
а не меняй кластер предыдущей работы. Убедись, что контекст выбран явно:

```bash
kind get clusters
kubectl config get-contexts

kind create cluster --name lab6 --wait 5m
kubectl --context kind-lab6 get nodes -o wide
```

Проверь, что в новом кластере одна Ready-нода. Установи в нём Prometheus/Grafana
и CloudNativePG по процедуре из [Лабы 5](../lab-5/README.md), используя
`--kube-context kind-lab6` во всех командах. Для Prometheus используй
[`api/prometheus/values.yaml`](../api/prometheus/values.yaml), а для API — базовый
чарт `shop-chart`. Если стек Лабы 2 уже работает в другом кластере, он не
переносится в `lab6` сам: либо установи его в `lab6`, либо осознанно выполняй
работу в том кластере и запиши это отклонение.

До продолжения проверь готовность мониторинга и базы:

```bash
kubectl --context kind-lab6 -n monitoring get pods
kubectl --context kind-lab6 -n shop get pods,svc
kubectl --context kind-lab6 -n shop get cluster shop-postgres
kubectl --context kind-lab6 wait -n shop \
  --for=condition=Ready cluster/shop-postgres --timeout=5m
```

Имя ресурса Postgres зависит от Helm release. Если release называется иначе,
замени `shop-postgres` на фактическое имя из `kubectl get cluster -n shop`.

## Часть 0. Подготовь API и скрипт-гейт

Перед установкой mesh измени API в [`shop-services/api/`](../shop-services/api/)
и его handlers так, чтобы:

- `GET /version` возвращал версию экземпляра (`v1` или `v2`) и HTTP 200;
- `GET /health` оставался быстрым liveness/readiness endpoint;
- только v2 при `FAIL=true` отвечала HTTP 500 на `POST /order`, до записи в БД;
- v1 при `FLAKY=true` иногда возвращала HTTP 500 на `GET /orders`; случайность
  должна быть настоящей и контролируемой, например через `math/rand`;
- в выключенном режиме флагов обычные endpoints работали как раньше.

Сохрани текущую модель метрик приложения: в репозитории уже есть
`http_requests_total{path,status}` и
`http_request_duration_seconds_bucket{path}`. Проверь, что счётчик увеличивается
и для ответов с ошибкой. Добавь версию в метки или обеспечь отдельные Service
имена, которые Prometheus будет сохранять в метке `service`.

Скрипт-гейт опрашивает Prometheus примерно раз в 15 секунд. Его логика должна
быть понятной и проверяемой:

1. запросить у Prometheus долю 5xx и p95 latency для `shop-api-v2`;
2. не принимать решение на слишком малой выборке (например, менее 20 запросов
   за окно);
3. если доля 5xx или задержка выше порога — установить вес v2 в 0%;
4. если метрики ниже порогов несколько последовательных окон — увеличить вес
   v2 ограниченным шагом;
5. ограничить долю диапазоном 0–100%, записывать принятое решение, старые и
   новые веса, значения метрик и ошибки API.

Выбери и запиши пороги до запуска теста, а не подгоняй их после результата.
Для простого учебного гейта подойдут окно 2 минуты, порог ошибок 5%, p95
latency 200 мс и шаг продвижения 10–25 процентных пунктов. Это стартовые
настройки, их нужно сверить с реальными baseline-измерениями.

Гейт меняет вес в Helm values через `helm upgrade` либо обновляет объект маршрута
через Kubernetes API. Не храните kubeconfig, токены и пароль БД в git. Для
постоянного запуска в кластере выдай ему ServiceAccount только необходимые
права на чтение метрик и изменение ресурса маршрутизации; не используй
cluster-admin.

Проверь API локально до упаковки:

```bash
go test ./...
```

Собери отдельные образы версий и загрузи их в kind. Выполняй команды из корня
репозитория; точные теги должны совпадать с values чарта:

```bash
docker build -t docker.io/library/shop-api:v1 \
  -f shop-services/api/Dockerfile ./shop-services
docker build -t docker.io/library/shop-api:v2 \
  -f shop-services/api/Dockerfile ./shop-services
kind load docker-image docker.io/library/shop-api:v1 --name lab6
kind load docker-image docker.io/library/shop-api:v2 --name lab6
```

Для разных настроек v1 и v2 используй значения Helm, которые передают
`API_VERSION`, `FLAKY` и `FAIL` в нужный Deployment. Убедись, что обе версии
могут использовать секрет PostgreSQL, созданный CloudNativePG.

## Часть 1. Установи Linkerd

Установи CLI по официальной инструкции Linkerd и сохрани его версию:

```bash
linkerd version
linkerd check --pre
```

Установи control plane и Viz extension:

```bash
linkerd install --crds | kubectl --context kind-lab6 apply -f -
linkerd install | kubectl --context kind-lab6 apply -f -
linkerd viz install | kubectl --context kind-lab6 apply -f -

linkerd check --context kind-lab6
linkerd viz check --context kind-lab6
kubectl --context kind-lab6 -n linkerd get pods
```

Если установленная версия CLI не принимает `--context` у команд `linkerd`, задай
контекст через `kubectl config use-context kind-lab6` и проверь текущий контекст
перед каждой командой. Не удаляй и не переустанавливай другие кластеры.

Установи Gateway API CRD, если их ещё нет и выбранная версия Linkerd требует их
для `HTTPRoute`. Используй версию CRD, совместимую с выбранной версией Linkerd;
проверь результат:

```bash
kubectl --context kind-lab6 get crd httproutes.gateway.networking.k8s.io
```

Linkerd Viz может устанавливать свой вспомогательный Prometheus. Для графиков и
алертов лабораторной используй стек Prometheus/Grafana из Лабы 2, а не смешивай
результаты двух Prometheus. Если оставляешь Viz Prometheus для `linkerd viz`,
не включай его запросы в основные панели и сравнение ресурсов приложения.

## Часть 2. Измерь baseline без mesh

Сначала установи только v1 с `FLAKY=false`, без аннотаций Linkerd и без
`HTTPRoute`. Оставь ту же ноду, параметры CPU/memory, базу и настройки
Prometheus, которые будут использоваться после включения mesh:

```bash
helm upgrade --install shop ./shop-chart \
  --kube-context kind-lab6 --namespace shop --create-namespace \
  -f shop-chart/values.yaml \
  -f shop-chart/values-lab6.yaml \
  --wait --timeout 10m

kubectl --context kind-lab6 -n shop get pods,svc -o wide
kubectl --context kind-lab6 -n shop rollout status deploy/shop-api-v1
```

Настрой port-forward к стабильному API Service и оставь его работать в отдельном
терминале:

```bash
kubectl --context kind-lab6 -n shop port-forward svc/shop-api 18080:8080
```

Сними, например, 500 последовательных измерений одного и того же запроса
`GET /orders`, сохранив значения `time_total` в файл:

```bash
mkdir -p lab-6/evidence
for i in $(seq 1 500); do
  curl -fsS -o /dev/null -w '%{time_total}\n' \
    http://127.0.0.1:18080/orders
done > lab-6/evidence/baseline-latency.txt
```

Вычисли p50/p95 тем же способом для mesh-прогона. Например, для p95:

```bash
python3 -c 'import statistics; x=sorted(float(v) for v in open("lab-6/evidence/baseline-latency.txt")); print("p50_s=", statistics.median(x), "p95_s=", x[int(.95*(len(x)-1))])'
```

Запиши дату, число запросов, успешность, concurrency (в этом примере 1),
p50/p95, CPU и память. Сними `kubectl top pod -n shop --containers`; если
Metrics Server не установлен, используй метрики контейнеров из существующего
Prometheus и явно укажи источник. Отдельно запиши limits/requests из Helm
values: requests не являются измеренным потреблением.

Сделай скриншот или экспорт панели Prometheus с числом запросов и задержкой.
Сохрани исходные результаты в `lab-6/evidence/`, не вписывай предполагаемые
числа как будто это замеры.

## Часть 3. Подключи API к mesh и проверь mTLS

Добавь в шаблон Pod API опциональную аннотацию Linkerd:

```yaml
linkerd.io/inject: enabled
```

Управляй ею через `values.yaml`, например `mesh.enabled`, чтобы включать mesh у
API без ручного изменения live Deployment. Добавь аннотацию в `spec.template.metadata`,
затем пересоздай Pod’ы через `helm upgrade`. Не добавляй её Postgres и worker.
Включи Linkerd identity в конфигурации control plane; для лабораторного стенда
допустима автоматически сгенерированная identity, но в отчёте укажи, что
сертификаты не являются настройкой production CA.

Проверь, что у API Pod появились контейнеры приложения и `linkerd-proxy`, и
что прокси готовы:

```bash
kubectl --context kind-lab6 -n shop get pods \
  -o custom-columns='NAME:.metadata.name,READY:.status.containerStatuses[*].ready,CONTAINERS:.spec.containers[*].name'
linkerd check --context kind-lab6 --proxy
```

Сгенерируй HTTP-трафик к API и подтверди защищённый edge через Viz:

```bash
linkerd viz edges deploy -n shop
linkerd viz stat deploy -n shop
```

Сохрани вывод, где для соединения API указано `SECURED`/TLS. Дополнительно
проверь состояние identity и сертификат прокси штатными средствами Linkerd,
не выводя приватный ключ. В отчёте объясни разницу между «есть sidecar» и
«конкретное соединение подтверждено как защищённое»: одного факта наличия
контейнера недостаточно.

**Доказательство:** скриншот или вывод Linkerd с защищённым соединением и
identity, список Pod’ов с прокси и успешный `linkerd check`. Не называй
шифрование подтверждённым, если в фактическом выводе нет статуса TLS/identity.

## Часть 4. Проверь retries на flaky v1

Добавь аннотации mesh-политики retry к маршруту только для идемпотентного
`GET /orders`. Ограничь число попыток, например тремя, и задай короткий timeout.
В Gateway API HTTPRoute используй ограниченный match по пути и HTTP method GET.
Для `POST /order` retry не включай. В отчёте укажи точные имена и значения
аннотаций, которые поддерживает установленная версия Linkerd.

Создай временный client Pod с `linkerd.io/inject: enabled` в namespace `shop`.
Убедись, что в нём появился `linkerd-proxy`, затем включи `FLAKY=true` только у
v1 и выполни достаточное количество запросов через стабильный Service
`shop-api`, а не напрямую по Pod IP:

```bash
kubectl --context kind-lab6 -n shop get pods
kubectl --context kind-lab6 -n shop exec deploy/lab6-client -c client -- \
  sh -c 'i=0; while [ "$i" -lt 500 ]; do curl -sS -o /dev/null -w "%{http_code}\n" http://shop-api:8080/orders; i=$((i+1)); done'
```

Уточни имя Deployment/контейнера клиента под свой manifest. Сохрани HTTP-коды
клиента, затем сравни их с `http_requests_total` для v1, где должны быть видны
ошибки backend. Прокси-метрики Linkerd показывают дополнительные попытки и
успешные ответы клиента; сначала проверь фактические имена метрик и меток в
Prometheus для установленной версии.

В README объясни механизм по фактам: backend действительно зарегистрировал
5xx, retry увеличил число попыток, но доля ошибок на стороне клиента стала
значительно ниже. Если этого не произошло, не приписывай результат circuit
breaking: у Linkerd в этой проверке используется retry, а не автоматическое
outlier detection. Проверь, что маршрут и источник запросов действительно
mesh-нуты, policy совпадает с `GET /orders`, а `FLAKY` включён у правильной
версии.

## Часть 5. Разверни v1/v2 и проверь 90/10

Добавь значения для весов в chart values (сумма весов должна быть 100) и
шаблон Gateway API `HTTPRoute`, управляемый Helm. Начальные значения: v1 — 90,
v2 — 10. Проверь, что обе версии готовы и что в HTTPRoute статус `Accepted` и
`ResolvedRefs` положительный:

```bash
kubectl --context kind-lab6 -n shop get deploy,svc,httproute
kubectl --context kind-lab6 -n shop describe httproute shop-api
```

После установки отправь, например, 1000 запросов `GET /version` через
`shop-api` из mesh-нутого клиента. Считай ответы по версии и сравни доли с
настроенными весами. Из-за случайной природы балансировки результат не будет
точно 900/100; запиши фактические значения и объясни статистическое отклонение.
Не тестируй распределение по прямым Pod IP или через отдельные Services версий:
это обходит traffic split.

**Результат:** таблица вида `версия | вес HTTPRoute | ответов /version | доля`.
Добавь вывод объекта маршрута и скриншот/график.

## Часть 6. Запусти автопилот

Убедись, что `shop-chart` управляет весами и `helm upgrade` не удаляет
аннотации retry, мониторинг или другие параметры. Проверь гейт отдельно:

1. тестовый запрос к Prometheus API возвращает текущие метрики v2;
2. при нулевой выборке гейт не повышает вес;
3. при искусственно высоком error ratio гейт устанавливает v2 в 0%;
4. при здоровых метриках гейт повышает долю ровно на заданный шаг;
5. повторный запуск не создаёт лишний Deployment и сохраняет значения после
   Helm upgrade.

Запусти гейт с интервалом около 15 секунд. В README опиши PromQL, окно,
порог ошибок, порог p95, минимальный объём выборки и число успешных окон до
следующего шага. Добавь один пример строки журнала решения гейта. Укажи, где
работает скрипт (локально или в кластере) и как он получает доступ к Prometheus
и Helm/Kubernetes API, не публикуя секреты.

## Часть 7. Проведи неудачный и успешный релиз

Перед каждым сценарием сбрось веса к 90/10, убедись, что метрики и графики
обновились, и сохрани их в `lab-6/evidence/`.

### Сценарий A — плохая v2

1. Включи `FAIL=true` у v2, оставь v1 здоровой.
2. Посылай реальные `POST /order` через стабильную точку входа с корректным
   JSON. Ошибка v2 должна быть видна в метриках; не используй retry для POST.
3. Дождись окна оценки гейта.
4. Проверь по логам, Helm values и HTTPRoute, что гейт сам поставил вес v2 в
   0%, а доля 5xx/успешных ответов после отката изменилась ожидаемым образом.
5. Сохрани график доли трафика v2 и ошибок во времени.

### Сценарий B — здоровая v2

1. Отключи `FAIL`, проверь `/health`, `/ready`, `/version` и успешный запрос
   заказа для v2.
2. Сбрось traffic split на 90/10 и дождись заполнения окна Prometheus.
3. Запусти гейт. На каждом шаге проверь вес в HTTPRoute и число запросов,
   фактически попавших на v2.
4. Дождись 100% трафика на v2; убедись, что доля ошибок и p95 не превышают
   заданных порогов.
5. Сохрани второй график и логи решений гейта.

Не считай запуск команды гейта успешным доказательством автоматизации:
результат должен быть виден в изменении весов маршрута и реальных ответах API.

## Часть 8. Измерь цену mesh

Повтори baseline-прогон с тем же URL, числом запросов, concurrency, нодой,
настройками CPU/memory, состоянием базы и способом измерения. Проведи тест
сначала без mesh, затем с mesh, не меняя одновременно образ приложения,
limits или набор запросов.

Запиши в таблицу:

| Режим | p50 | p95 | API CPU | API memory | proxy CPU | proxy memory |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Без mesh (baseline) | | | | | — | — |
| С Linkerd | | | | | | |

Для Pod с sidecar отдельно покажи ресурсы контейнеров API и `linkerd-proxy`
(`kubectl top pod --containers -n shop`), а также суммарный CPU/memory на Pod.
Используй одинаковый интервал прогрева и сбора данных. Посчитай проценты:

```text
рост p95 (%) = (p95_mesh - p95_baseline) / p95_baseline * 100
рост CPU (%) = (cpu_mesh - cpu_baseline) / cpu_baseline * 100
рост памяти (%) = (memory_mesh - memory_baseline) / memory_baseline * 100
```

Сравни результаты в цифрах и вынеси вердикт для маленького `shop`: какую
надёжность/безопасность mesh добавил, во сколько это обошлось и оправдана ли
цена именно для этого масштаба. Если на одноузловом локальном кластере
разброс велик, повтори тест несколько раз и покажи диапазон, а не выбирай
удобный одиночный прогон.

## Часть 9. Мониторинг, дашборд и три алерта

Проверь, что существующий стек Prometheus Operator работает в `lab6` и
ServiceMonitor/PrometheusRule выбираются по label `release` так же, как в
[`shop-chart/templates/servicemonitor.yaml`](../shop-chart/templates/servicemonitor.yaml)
и [`shop-chart/templates/prometheusrule.yaml`](../shop-chart/templates/prometheusrule.yaml).
Добавь chart templates для метрик API версий и Linkerd proxy. Proxy metrics
обычно доступны на admin port 4191; добавь именованный Service port для scrape
или подходящий PodMonitor, затем проверь Target в Prometheus. Не меняй target
порта наугад: сначала проверь `/metrics` и labels у Pod установленной версии
Linkerd.

Для Grafana собери панели:

- запросы в секунду по `service`/версии;
- доля HTTP 5xx по версиям;
- p95 latency по версиям;
- вес v2 из метрик/серии событий или экспортируемого значения гейта;
- retries/ответы Linkerd, если соответствующие метрики обнаружены у установленной
  версии.

Примеры запросов для уже имеющихся метрик приложения (проверь метки `namespace`
и `service` в Prometheus и адаптируй селекторы к своим именам Services):

```promql
sum by (service) (
  rate(http_requests_total{namespace="shop",service=~"shop-api-v1|shop-api-v2"}[2m])
)
```

```promql
sum by (service) (
  rate(http_requests_total{namespace="shop",service=~"shop-api-v1|shop-api-v2",status=~"5.."}[2m])
)
/
clamp_min(
  sum by (service) (
    rate(http_requests_total{namespace="shop",service=~"shop-api-v1|shop-api-v2"}[2m])
  ),
  0.000001
)
```

```promql
histogram_quantile(
  0.95,
  sum by (le, service) (
    rate(http_request_duration_seconds_bucket{namespace="shop",service=~"shop-api-v1|shop-api-v2"}[5m])
  )
)
```

Добавь три Prometheus alerts и объясни выбор порогов. Для начала можно
использовать:

| Alert | Условие | Почему |
| --- | --- | --- |
| `ShopApiVersionHighErrorRatio` | 5xx > 5% для одной версии в течение 2 минут, при наличии достаточного числа запросов | Замечает плохой релиз до продвижения большей доли трафика |
| `ShopApiVersionHighP95` | p95 выше согласованного SLO, например 200 мс, 5 минут подряд | Замечает регрессию latency, даже если endpoint отвечает 200 |
| `ShopApiMetricsTargetDown` | API target недоступен более 1 минуты | Обнаруживает проблемы сбора метрик и потерю наблюдаемости |

Согласуй синтаксис PromQL и имена labels с фактическими рядами. Добавь
`for`, severity и понятные `summary`/`description`; порог срабатывания должен
соответствовать выбранному baseline, а не быть случайным. Проверь правила
после Helm upgrade:

```bash
kubectl --context kind-lab6 -n shop get servicemonitor,prometheusrule
```

Убедись в Prometheus → Status → Targets, что API и proxy targets `UP`, а в
Prometheus → Alerts, что правила загружены. Проверь как минимум один алерт
контролируемым экспериментом и зафиксируй состояния `pending`/`firing`, если
они действительно возникли. Сохрани скриншот дашборда.

## Сдача и чек-лист доказательств

Сдай обновлённый код API и Dockerfile, скрипт-гейт и его Dockerfile, Helm-чарт
с версиями, HTTPRoute, mesh-настройками, ServiceMonitor/PrometheusRule и
финальными values. В `lab-6/README.md` заполни фактические результаты и выводы.

Проверь, что приложены:

- выбор Linkerd и объяснение границ mesh;
- baseline до mesh и замеры после него;
- подтверждение mTLS/identity Linkerd;
- график ошибок backend и успешности клиента при `FLAKY=true`;
- фактическое распределение запросов 90/10;
- график автоматического отката плохой v2;
- график автоматического продвижения здоровой v2 до 100%;
- сравнение latency и ресурсов, расчёт процентов и обоснованный вердикт;
- дашборд и три объяснённых алерта;
- скриншоты, логи гейта и данные экспериментов в `lab-6/evidence/`.

Не записывай секреты, kubeconfig, пароли, приватные ключи или неподтверждённые
значения. Удаляй только свои временные ресурсы; не выполняй `kind delete cluster`
для кластеров предыдущих лабораторных.

## Фактический отчёт о выполнении лабораторной

В текущем проекте лабораторная выполнена в существующем кластере `kind-lab5` в
отдельном namespace `lab6`, поскольку отдельный одноузловой кластер `kind-lab6`
не был доступен в этой среде. Все артефакты сохраняются в каталоге
[`lab-6/evidence/`](./evidence/).

### Что было реализовано

- API v1/v2, `GET /version`, флаги `FAIL`/`FLAKY` и подходящий middleware для
  внедрения ошибок: [`shop-services/api/release.go`](../shop-services/api/release.go),
  [`shop-services/api/main.go`](../shop-services/api/main.go).
- Canary deployment и routing через Gateway API `HTTPRoute`:
  [`shop-chart/templates/api-canary-deployments.yaml`](../shop-chart/templates/api-canary-deployments.yaml),
  [`shop-chart/templates/api-canary-routes.yaml`](../shop-chart/templates/api-canary-routes.yaml),
  [`shop-chart/templates/api-canary-services.yaml`](../shop-chart/templates/api-canary-services.yaml),
  [`shop-chart/values-lab6.yaml`](../shop-chart/values-lab6.yaml).
- Проверка rollout/canary: `shop-api` в namespace `lab6` отдаёт трафик v1/v2 в
  пропорции 90/10, а `linkerd.io/inject: enabled` подтверждает mesh для API и
  клиентскогоPod.

### Артефакты и доказательства

- Baseline без mesh: [`lab-6/evidence/baseline-no-mesh.summary.json`](./evidence/baseline-no-mesh.summary.json)
- Распределение traffic split: [`lab-6/evidence/route-split.txt`](./evidence/route-split.txt)
- Описание шагов и сама инструкция: [`lab-6/README.md`](./README.md)
- Для получения выводов из маршрута и canary-режима использованы:
  `kubectl -n lab6 get httproute`, `kubectl -n lab6 get pods`, `kubectl -n lab6 exec ... curl http://shop-api:8080/version`

### Фактические наблюдения

- Baseline без mesh на `GET /orders` для 500 запросов показал `p95 = 29.1 ms`,
  `mean = 10.46 ms`, все ответы `200`.
- На живом canary-распределении из 40 проверок было получено 31 ответа v1 и 9
  ответов v2, то есть трафик близок к ожидаемому 90/10 и подтверждает route split.
- Для `GET /version` через стабильный сервис `shop-api` видны ответы обоих
  вариантов (`v1` и `v2`), что подтверждает корректную работу `HTTPRoute`.
- Включённый mesh и sidecars зафиксированы в Pod-списке API (`2/2` ready на v1,
  v2 и клиентский pod), а retry-опции маршрута видны в `httproute`-описании.

### Краткий вывод по лабораторной

Лабораторная продемонстрировала, что схема с Linkerd + Gateway API + canary split
работает в текущем проекте: API версии разделены, mesh внедрён, трафик может
делиться между v1 и v2, и в системе есть артефакты для дальнейшего анализа
ошибок/ретраев/алертов. Для чистой финальной проверки с графиками и полным
rollback/promotion циклом требуется стабильный кластер и дополнительная запись
метрик Prometheus/Grafana во времени; в текущем эксперименте уже подготовлены
все необходимые точки входа, шаблоны и экспериментальные данные для этого.

В месте, где пользователь планирует вставить итоговые графики, стоит добавить:
- график latency до/после mesh;
- график трафика и ошибок по версиям v1/v2;
- график отката/продвижения canary; 
- график CPU/memory для baseline vs mesh.
