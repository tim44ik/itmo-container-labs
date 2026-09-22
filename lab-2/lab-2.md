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

Чтобы Kubernetes увидел твой собственный образ сервиса без загрузки его в интернет, собирать его нужно напрямую внутри виртуальной среды Minikube. Для этого нужно связать терминал с демоном кластера:
```sh
eval $(minikube docker-env)
```

Далее необходимо собрать образ внутри кластера и мы готовы приступать к 1 части:
```bash
DOCKER_BUILDKIT=0 docker build -t api:v1 .
```

Используется `DOCKER_BUILDKIT=0`, так как без него вылетала ошибка:
```ERROR: failed to build: failed to inspect pulled image moby/buildkit:buildx-stable-1: Error response from daemon: 404 page not found```
