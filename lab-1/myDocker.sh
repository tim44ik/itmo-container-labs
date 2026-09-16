#!/usr/bin/env bash

PID="${1:-7422}"

echo "Часть 2. Изоляция пространств имен"

echo "=== ps aux | grep api ==="
ps aux | grep -E "[a]pi"

echo
echo "=== sudo nsenter --pid --mount --target $PID ps aux ==="
sudo nsenter --pid --mount --target "$PID" ps aux

echo
echo "=== hostname ==="
hostname

echo
echo "=== sudo nsenter --uts --target $PID hostname ==="
sudo nsenter --uts --target "$PID" hostname

echo
echo "=== ip addr ==="
ip addr

echo
echo "=== sudo nsenter --net --target $PID ip addr ==="
sudo nsenter --net --target "$PID" ip addr

echo
echo "=== sudo nsenter --user --target $PID id ==="
sudo nsenter --user --target "$PID" id

echo
echo "=== ps -o pid,uid,user -p $PID ==="
ps -o pid,uid,user -p "$PID"

echo
echo "=== sudo ls -l /proc/$PID/ns/mnt ==="
sudo ls -l "/proc/$PID/ns/mnt"

echo
echo "=== ls -l /proc/self/ns/mnt ==="
ls -l /proc/self/ns/mnt

echo
echo "=== ls -l /proc/self/ns/ipc ==="
ls -l /proc/self/ns/ipc

echo
echo "=== sudo ls -l /proc/$PID/ns/ipc ==="
sudo ls -l "/proc/$PID/ns/ipc"

echo "Часть 3. Изоляция ресурсов"