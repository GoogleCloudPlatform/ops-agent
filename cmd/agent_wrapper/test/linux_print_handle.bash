#!/bin/bash
BYTES=$1
trap "" SIGTERM SIGINT
for i in `seq 1 $BYTES`
do
  printf "a"
  sleep 0.1
done