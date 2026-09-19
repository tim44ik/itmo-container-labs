#!/bin/bash

if [ "$EUID" -ne 0 ]; then
  echo "Пожалуйста, запустите скрипт через sudo: sudo $0"
  exit 1
fi

CGROUP_NAME="lab-api"
MEMORY_LIMIT="20M"
PIDS_LIMIT="20"
CPU_LIMIT="30000 100000"

echo "[MyDocker] Шаг 1: Настройка подсистем cgroups v2..."
mkdir -p /sys/fs/cgroup/$CGROUP_NAME
echo "+memory +cpu +pids" > /sys/fs/cgroup/cgroup.subtree_control

echo "[MyDocker] Шаг 2: Установка жестких лимитов ресурсов..."
echo "$MEMORY_LIMIT" > /sys/fs/cgroup/$CGROUP_NAME/memory.max
echo "$PIDS_LIMIT" > /sys/fs/cgroup/$CGROUP_NAME/pids.max
echo "$CPU_LIMIT" > /sys/fs/cgroup/$CGROUP_NAME/cpu.max

echo "[MyDocker] Шаг 3: Генерация BPF-фильтра Seccomp..."
enosys -s mkdirat:EPERM -d > /tmp/mydocker_seccomp.bpf

echo "[MyDocker] Шаг 4: Запуск изолированной среды (unshare + capsh + setpriv)..."
echo "[MyDocker] Сервис api поднимается..."

unshare --pid --mount --net --uts --ipc --user --map-root-user --fork --mount-proc bash -c "
    echo \$\$ > /sys/fs/cgroup/$CGROUP_NAME/cgroup.procs
    ip link set lo up
    exec capsh --caps='' -- -c \"
        exec setpriv --no-new-privs --seccomp-filter /tmp/mydocker_seccomp.bpf ../../api/api
    \"
"
