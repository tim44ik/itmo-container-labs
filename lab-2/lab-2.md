## Лабораторная работа 2
___
Целью работы является поднять полный стек мониторинга, настроить трейсинг и алерты.
### Часть 0. Подготовка
___
Для начала, добавим в код стандартных ручек телеметрию.
Затем, подредактируем Dockerfile, добавив в него:
```Dockerfile
COPY go.mod go.sum ./
RUN go mod download
```

И соберем образ:
```bash
sudo docker build -t api:v1 .
```

Теперь надо настроить Кубер-кластер. Для выполнения работы я выбрал minikube:
Установим Kubernetes и Minikube, а затем запустим с драйвером Docker:
```sh
minikube start --vm-driver=docker
```

Добавим аддон ingress, чтобы в дальнейших частях лабы открывать дашборды Grafana, Jaeger и Karma прямо через браузер на хосте:
```sh
minikube addons enable ingress
```

Теперь просто загрузим уже собранный ранее образ:
```sh
minikube image load api-multi:latest
```

Далее установим Helm и создадим чарт для нашего сервиса:
```sh
helm create api-chart
```

Удалим стандартные шаблоны и создадим наш:
```sh
rm -rf api-chart/templates/*
nano api-chart/templates/api.yaml
```

Содержимое манифеста можно посмотреть [здесь](../api/api-chart/templates/api.yaml)

Установим наш сервис:
```sh
helm install my-api ./api-chart
```

Получим:
```sh
NAME: my-api
LAST DEPLOYED: Sat Sep 26 01:05:48 2026
NAMESPACE: default
STATUS: deployed
REVISION: 1
DESCRIPTION: Install complete
TEST SUITE: None
```