# Вспомогательные файлы для лабы 3, части 2

Используются вместе с пошаговыми командами в чате. Команды выполняются из корня репозитория.
Файлы подготовлены, но не запускались в кластере пользователя.

- postgres-temp.json: временная PostgreSQL 16 в namespace shop. Используется официальный
  образ postgres:16-bookworm с UID/GID 999; writable emptyDir для PGDATA, сокета и /tmp.
  База нужна только для опытов части 2, не выполняет часть 3 про оператор.
  Удаление/пересоздание Pod БД уничтожает данные. До части 3 Pod БД не удаляйте.
  В учебном внутреннем соединении TLS отключён явно; пароль создаётся случайным.
- create-db-secret.py: создаёт shop-db, не печатает пароль, не перезаписывает существующий Secret.
- migrate.json: одноразовый Job /api migrate, создающий таблицу orders.
- health-load.json: запросы к Service shop-api из отдельного Pod примерно 5 раз/с.
  Успех wget обозначается OK, ошибка HTTP/сети — FAIL. Это выборочная проверка /health,
  а не доказательство доступности всех маршрутов при любой нагрузке.
- set-values.py: изменяет api.replicas, api.image.tag, api.healthFail в values.yaml.
- prepare-api-shutdown.py: добавляет API preStop sleep 5s без shell в scratch-образе.
  Нужен Kubernetes >=1.30; используемый кластер 1.37 подходит.
- count-health.py: считает OK/FAIL в сохранённых логах нагрузки.

Часть 2 завершена после живых проверок: 3 API/2 worker; создание/обработка заказа;
восстановление удалённого API Pod; масштабирование через values и helm upgrade;
rolling update v1->v2 с наблюдением подов и логом запросов; HEALTH_FAIL=true,
неготовый новый Pod при сохранении рабочих старых; helm rollback на успешную ревизию.

v2 в инструкции — отдельная сборка с новой меткой образа lab.version=v2:
код приложения не меняется, но конфигурация образа и его ID отличаются.
Политики требуют Enforce, пробы используют /health. Этот health не проверяет БД;
работу PostgreSQL проверяют отдельные POST /order и GET /orders.

Ресурсы временной БД и нагрузочного Pod — начальные значения, не результаты krr.
Данные PostgreSQL, миграция и временный клиент не входят в Helm release shop.
В части 3 PostgreSQL заменяется объектом оператора и добавляется в чарт.

Источники:
- https://kubernetes.io/docs/concepts/workloads/controllers/deployment/
- https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/
- https://hub.docker.com/_/postgres
- https://helm.sh/docs/helm/helm_rollback/
