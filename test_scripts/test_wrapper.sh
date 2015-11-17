#!/usr/bin/env bash

echo "Testing how proc_box handles process trees"

# Compile the c program up
gcc -o mem_test ./test_scripts/memory.c

./mem_test &
MEM_PID_1=$!
echo $MEM_PID_1

./mem_test &
MEM_PID_2=$!
echo $MEM_PID_2

sleep 60

kill $MEM_PID_1
kill $MEM_PID_2

# Clean up the compiled binary
rm mem_test
