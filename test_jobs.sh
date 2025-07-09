#!/bin/bash

API_URL="http://localhost:8080/run"

declare -a JOBS=(
  '{"name":"EchoHello", "cmd":"echo Hello"}'
  '{"name":"ListDir", "cmd":"ls -la"}'
  '{"name":"Sleep3Sec", "cmd":"sleep 3 && echo Done sleeping"}'
  '{"name":"InvalidCmd", "cmd":"foobar123"}'
  '{"name":"ShowDate", "cmd":"date"}'
)

for job in "${JOBS[@]}"; do
  echo "Submitting job: $job"
  curl -s -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d "$job"
  echo -e "\n---"
done
