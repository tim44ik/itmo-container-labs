# Лаба 3, часть 1 — политики Kyverno

Выбран Kyverno: политики описываются YAML-манифестами Kubernetes, без изучения Rego.
Движок нужно установить отдельно, до применения этих ClusterPolicy и до установки shop.
В архиве нет установщика Kyverno. Не применяйте всю папку policies рекурсивно:
audit и enforce содержат разные режимы одних и тех же политик.

| Правило | Польза | Нарушающий файл |
|---|---|---|
| require-resources | Планировщик видит запросы CPU/памяти; контейнеры имеют лимиты | 01-resources.yaml |
| trusted-registries | Образы разрешены только из перечисленных реестров | 02-registries.yaml |
| require-labels | У подов есть метки для поиска и сбора логов | 03-labels.yaml |
| deny-privileged | Запрещён privileged-режим контейнера | 04-privileged.yaml |
| readonly-rootfs | Контейнер не может менять свою корневую файловую систему | 05-readonly.yaml |

ClusterPolicy не ограничены namespace shop: они охватывают поды во всех namespace,
которые попадают в admission/background-проверки Kyverno. Собственные исключения
движка (его namespace, resourceFilters и webhook selectors) также влияют на охват.
Автогенерация Kyverno распространяет подходящие Pod-правила на контроллеры,
включая Deployment. Фактический охват проверяется после установки движка.

Реестры docker.io, ghcr.io, quay.io и registry.k8s.io — учебный allowlist,
а не гарантия безопасности всех размещённых там образов и не проверка подписей.
Для production список обычно сужают до реестра/репозитория организации.
Локальные shop-образы имеют полное имя docker.io/library/shop-api:v1 и
docker.io/library/shop-worker:v1; публиковать их в Docker Hub не требуется.
Нужно собрать и загрузить их в kind под теми же именами.

resources проверяются у обычных и init-контейнеров. У ephemeral-контейнеров
resources не задаются: для них применяются правила образов и securityContext.
Пятое правило — readOnlyRootFilesystem; запуск не от root и сброс capabilities
дополнительно заложены в чарт, но не считаются отдельными правилами.

## Порядок применения ПОСЛЕ установки Kyverno

В существующем кластере сначала используйте Audit: мониторинг и системные
компоненты могут не соответствовать этим правилам. Audit только фиксирует
нарушения, не выполняет требование лабораторной об отклонении манифестов.

```bash
kubectl --context kind-labs apply -f policies/audit/
kubectl --context kind-labs get policyreports -A
kubectl --context kind-labs get clusterpolicyreports
```

Перед Enforce нужно разобрать отчёты и привести затронутые компоненты к правилам.
Enforce не удаляет существующие поды, но может заблокировать их пересоздание
и обновления, в том числе у мониторинга. Исключения нельзя выдавать за полный
охват кластера: если понадобятся, нужно отдельно описать их в отчёте.

Когда предварительная проверка завершена:

```bash
kubectl --context kind-labs create namespace shop --dry-run=client -o yaml | kubectl --context kind-labs apply -f -
kubectl --context kind-labs apply -f policies/enforce/
kubectl --context kind-labs get clusterpolicies
kubectl --context kind-labs create --dry-run=server -f policies/examples/valid-pod.yaml
for manifest in policies/violations/*.yaml; do
  echo "Checking $manifest"
  kubectl --context kind-labs create --dry-run=server -f "$manifest"
done
```

Ожидается: контрольный Pod принят в dry-run; каждый из пяти нарушающих
манифестов отклонён соответствующей политикой Kyverno. dry-run=server проходит
admission, но не сохраняет Pod. Ошибка скачивания образа или отсутствие CRD —
не доказательство работы правил. Если на namespace включён Pod Security
Admission, privileged может быть отклонён им: для отчёта различайте источники.

Скриншоты: список политик и по одному отказу с именем правила для каждого файла.
До запуска не пишите в отчёте, что отклонения уже получены.

Официальные примеры, на которые опирается структура правил:
- https://kyverno.io/policies/best-practices/require-pod-requests-limits/require-pod-requests-limits/
- https://kyverno.io/policies/best-practices/require-ro-rootfs/require-ro-rootfs/
- https://kyverno.io/policies/
