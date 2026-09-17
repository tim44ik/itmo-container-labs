## Лабораторная работа 1
___
### Часть 0. Код
___
Написал на Go, потому что самый релевантный опыт был связан с ним.
Код можно посмотреть (здесь)[/api]
### Часть 1. Запуск
___
Соберем бинарник:
```bash
go build -o api
```
Запустим сервис:
```bash
./api
```
Сервис запустится по адресу localhost:8080. Проверяем эндпоинт /health:
![скрин ответа от /health](screenshots/1.png)

Далее посмотрим номер процесса при помощи команды `ps`:
![номер процесса](screenshots/2.png)

Здесь PID = 7262
### Часть 2. Выделение пространства имен
Выделим для нашего сервиса отдельное пространство имен:

```bash
unshare --pid --mount --net --uts --ipc --user --map-root-user --fork --mount-proc ./api
```
Найдем процесс в общем пространстве:
![номер процесса в общем пространстве](screenshots/3.png)

Здесь PID = 7422

Найдем этот же процесс в выделенном пространстве имен:
```bash
sudo nsenter --pid --target $PID ps -o pid,comm
```
![номер процесса в выделенном пространстве](screenshots/4.png)

А внутри хоста PID=1

Изменим внутреннее название хоста контейнера для наглядности:
```bash
sudo nsenter --uts --target $PID hostname api
```

Теперь хост внутри контейнера называется api, проверим название внутреннего и внешнего хоста:
![имя хоста](screenshots/5.png)

Далее проверим разделение сетевого пространства:
![интернет интерфейсы в общем и выделенном прострастве](screenshots/6.png)

Как можно заметить, интерфейсы и их количество отличаются, что доказывает наличие изоляции.

Проверим пространство имен пользователя:
![определение пользователя в общем и выделенном прострастве](screenshots/7.png)

Пользователь внутри имеет uid=0 и имя root, а снаружи uid=1000 и имя w

Проверим изоляцию дискового пространства:
![изоляция дискового пространства](screenshots/8.png?raw=true)

Сравнение ID mount namespace через /proc/7422/ns/mnt и /proc/self/ns/mnt показывает разные inode контейнера и хоста. Это доказывает изоляцию mount namespace. /proc/mounts внутри отсутствовал, так как /proc не был перемонтирован при создании namespace через unshare, поэтому для проверки использовалось сравнение inode namespace-симлинков.

Таким же образом проверим изоляцию ipcs:
![изоляция ipcs](screenshots/9.png)

Снова пришлось смотреть по id ноды, потому что таблицы были пусты в обоих пространствах.

### Часть 3. Лимиты
Первично нужно создать cgroup:
```bash
sudo mkdir /sys/fs/cgroup/lab-api
echo "+memory +cpu +pids"| sudo tee /sys/fs/cgroup/cgroup.subtree_control
```

Поместим процесс в cgroup:
```bash
echo $PID | sudo tee /sys/fs/cgroup/lab-api/cgroup.procs
```

Зададим лимит памяти в 10 мегабайт:
```bash
echo "10M" | sudo tee /sys/fs/cgroup/lab-api/memory.max
```
*примерно здесь крашнулась виртуалка, поэтому PID дальше будет 13103*

Поднимаем lo:
```bash
sudo nsenter --net --target $PID ip link set lo up
```

Теперь попробуем поймать OOM:
```bash
sudo nsenter --pid --mount --net --target $PID bash -c 'exec 3<>/dev/tcp/127.0.0.1/8080; echo -e "GET /eat?mb=50 HTTP/1.0\r\n\r\n" >&3; cat <&3'
```
![OOM](screenshots/10.png)

*После перезапуска PID=13934*

Поставим ограничение в 0,3 ядра:
```bash
echo "30000 100000" | sudo tee /sys/fs/cgroup/lab-api/cpu.max
```

Снова поднимаем сеть и дергаем за burn:
```bash
sudo nsenter --pid --mount --net --target  bash -c 'exec 3<>/dev/tcp/127.0.0.1/8080; echo -e "GET /burn?threads=1 HTTP/1.0\r\n\r\n" >&3; cat <&3 &'
```

Замечаем троттлинг:
![throttling](screenshots/11.png)

Задаем лимит процессов:
```bash
echo "15" | sudo tee /sys/fs/cgroup/lab-api/pids.max
```

Запустим форк бомбу через stress-ng:
```bash
sudo nsenter --pid --mount --net --target $PID bash -c 'stress-ng --fork 50'
```

Получаем ошибку "Ресурс временно недоступен"
![Ресурс временно недоступен](screenshots/12.png)

