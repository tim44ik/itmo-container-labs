# Лаба 3, часть 2 — чарт API и worker

В чарте два Deployment и два Service. API по умолчанию имеет 3 реплики,
worker — 2. Service worker нужен для HTTP health/metrics, не для очереди:
заказы worker читает непосредственно из PostgreSQL.

Обе пробы используют /health, как требует задание. HEALTH_FAIL=true вызывает
503 и провал readiness/liveness. /health намеренно не проверяет базу:
Ready в этой части не доказывает успешную запись заказа. Для проверки БД
у приложения есть /ready, а полная проверка — POST /order и GET /orders.

maxUnavailable: 0 сохраняет доступные реплики, maxSurge: 1 разрешает один
дополнительный Pod. minReadySeconds: 5 требует устойчивой готовности.
Это настройки для эксперимента, а не доказательство отсутствия ошибок:
rolling update и rollback ещё нужно выполнить на работающем кластере.

Все Pod-шаблоны содержат метки app и app.kubernetes.io/name, ресурсы,
полное имя разрешённого реестра, privileged: false, readOnlyRootFilesystem: true.
Дополнительно включены non-root, RuntimeDefault, drop ALL и запрет эскалации.
На корневую файловую систему эти сервисы писать не должны.

## Предварительные условия установки

1. Kyverno установлен, политики части 1 проверены и переведены в Enforce.
2. Образы собраны и загружены в kind под именами из values.yaml.
3. В namespace shop есть доступная PostgreSQL и Secret shop-db с ключом uri,
   содержащим полный DATABASE_URL. Название Secret и ключ настраиваются в values.
4. Схема БД создана командой `/api migrate` из образа API с этим DATABASE_URL.
   Автоматическая миграция при старте API не выполняется.

PostgreSQL, Secret и Job миграции в этот чарт пока НЕ входят: это отдельный
следующий этап. Без Secret поды получат CreateContainerConfigError.
Без доступной БД и таблицы orders health может быть зелёным, но заказы не работают.
Не создавайте фиктивный DATABASE_URL ради успешного helm --wait.

## Сборка (из корня репозитория в Ubuntu)

```bash
docker build -t docker.io/library/shop-api:v1 -f shop-services/api/Dockerfile ./shop-services
docker build -t docker.io/library/shop-worker:v1 -f shop-services/worker/Dockerfile ./shop-services
kind load docker-image docker.io/library/shop-api:v1 docker.io/library/shop-worker:v1 --name labs
```

Если старые shop-api:v1 и shop-worker:v1 уже собраны из этих исходников,
docker.io/library — их канонический префикс, повторная сборка не обязательна.

Установка ТОЛЬКО после выполнения предварительных условий:

```bash
helm upgrade --install shop ./shop-chart --kube-context kind-labs --namespace shop --create-namespace --wait --timeout 5m
kubectl --context kind-labs get deployments,pods,services -n shop
```

Редактируйте api.replicas/worker.replicas в values.yaml для изменения количества.
Для сломанного обновления меняйте api.healthFail на true.
Для новой версии меняйте api.image.tag и предварительно загружайте этот образ в kind.
Не перезаписывайте v1 новым содержимым: разные версии должны иметь разные теги.

При HEALTH_FAIL=true ожидается неуспешный rollout; старые готовые поды сохраняются.
Без --atomic Helm не откатывает такую установку автоматически: откат делается
отдельно через helm rollback на номер предыдущей успешной ревизии.

В этой поставке не выполнялись helm lint/template, сборка Docker, установка
в Kubernetes и испытания политик — по просьбе пользователя файлы выданы без проверок.
Предыдущие проверки Go-исходников перечислены отдельно в shop-services/VERIFICATION.txt.

В отчёте отделите подготовленные конфигурации от выполненных опытов.
Скриншоты после запуска: 3 API и 2 worker Ready; затем восстановление удалённого
Pod, масштабирование, rolling update, зависший rollout и результат rollback.
