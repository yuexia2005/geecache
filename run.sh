#!/bin/bash
trap "rm server;kill 0" EXIT

go build -o server ./cmd/gee
./server -port=8001 &
./server -port=8002 &
./server -port=8003 -api=1 &

sleep 2
echo ">>> start test"
curl "http://localhost:8888/api?key=Tom" -w "\n" &
curl "http://localhost:8888/api?key=Tom" -w "\n" &
curl "http://localhost:8888/api?key=Tom" -w "\n" &

wait
