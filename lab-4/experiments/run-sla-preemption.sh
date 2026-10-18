#!/usr/bin/env bash
set -euo pipefail

cd ~/container-labs/repo

# Повторный запуск не создаст второй тест.
if docker container inspect lab4-sla-preemption >/dev/null 2>&1; then
  echo "Контейнер lab4-sla-preemption уже существует. Запуск отменён."
  exit 1
fi

# Сохраняем предыдущий успешный тест отдельно.
mkdir -p lab-4/evidence/sla-before-preemption
docker logs lab4-sla-test \
  > lab-4/evidence/sla-before-preemption/k6.txt 2>&1
docker inspect lab4-sla-test \
  --format 'Started={{.State.StartedAt}} Finished={{.State.FinishedAt}} ExitCode={{.State.ExitCode}}' \
  > lab-4/evidence/sla-before-preemption/timing.txt
cp lab-4/evidence/sla-run-id.txt \
  lab-4/evidence/sla-before-preemption/run-id.txt

kubectl --context kind-labs delete pod shop-preemption-probe \
  -n shop --ignore-not-found --wait=true

echo "Ожидаем восстановления шести работающих batch-подов..."
for attempt in $(seq 1 60); do
  ready=$(kubectl --context kind-labs get deployment shop-batch \
    -n shop -o jsonpath='{.status.readyReplicas}')
  if [ "$ready" = "6" ]; then
    break
  fi
  sleep 5
done
if [ "$ready" != "6" ]; then
  echo "Batch не восстановился до 6 готовых реплик. Запуск отменён."
  exit 1
fi

pending=$(kubectl --context kind-labs exec \
  -n shop shop-postgres-1 -c postgres -- \
  psql -U postgres -d shop -Atc \
  "SELECT count(*) FROM public.orders WHERE status='pending';")
if [ "$pending" != "0" ]; then
  echo "В очереди $pending заказов. Дождись обработки и повтори запуск."
  exit 1
fi

api_ip=$(kubectl --context kind-labs get service shop-api \
  -n shop -o jsonpath='{.spec.clusterIP}')
run_id="lab4-sla-preemption-$(date +%s)"

docker run -d \
  --name lab4-sla-preemption \
  --network container:labs-worker \
  --mount type=bind,src="$(pwd)/lab-4/load/orders.js",dst=/scripts/orders.js,readonly \
  -e BASE_URL="http://$api_ip:8080" \
  -e RUN_ID="$run_id" \
  -e RATE=5 \
  -e DURATION=15m \
  "$(cat lab-4/k6-image.txt)" \
  run /scripts/orders.js

printf '%s\n' "$run_id" > lab-4/evidence/sla-run-id.txt
docker inspect lab4-sla-preemption --format '{{.State.StartedAt}}' \
  > lab-4/evidence/sla-start.txt

echo "k6 запущен. Через 60 секунд probe будет создан автоматически."
sleep 60

if [ "$(docker inspect lab4-sla-preemption --format '{{.State.Running}}')" != "true" ]; then
  docker logs lab4-sla-preemption
  echo "k6 завершился раньше времени. Probe не создаём."
  exit 1
fi

kubectl --context kind-labs apply \
  -f lab-4/experiments/preemption-probe.yaml

kubectl --context kind-labs wait \
  --for=condition=Ready pod/shop-preemption-probe \
  -n shop --timeout=3m

kubectl --context kind-labs get pod shop-preemption-probe \
  -n shop -o json \
  > lab-4/evidence/sla-probe.json

kubectl --context kind-labs get events -n shop \
  --field-selector reason=Preempted \
  -o yaml > lab-4/evidence/sla-preemption-events.yaml

kubectl --context kind-labs get deployments -n shop \
  | tee lab-4/evidence/sla-deployments-during.txt

echo "Probe готов. Ожидаем завершения k6; ничего вводить не нужно."
docker wait lab4-sla-preemption \
  > lab-4/evidence/k6-sla-exit-code.txt

docker logs lab4-sla-preemption \
  > lab-4/evidence/k6-sla.txt 2>&1
docker inspect lab4-sla-preemption --format '{{.State.FinishedAt}}' \
  > lab-4/evidence/sla-end.txt

echo "Тест завершён. Код выхода:"
cat lab-4/evidence/k6-sla-exit-code.txt
tail -55 lab-4/evidence/k6-sla.txt
