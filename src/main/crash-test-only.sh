#!/usr/bin/env bash

RACE=
TIMEOUT=timeout
TIMEOUT2="$TIMEOUT -k 2s 120s"
SOCKNAME=/var/tmp/5840-mr-`id -u`

rm -rf mr-tmp
mkdir mr-tmp || exit 1
cd mr-tmp || exit 1
rm -f mr-*

echo '***' Starting crash test.

# 生成正确输出
../mrsequential ../../mrapps/nocrash.so ../pg*txt || exit 1
sort mr-out-0 > mr-correct-crash.txt
rm -f mr-out*

rm -f mr-done

($TIMEOUT2 ../mrcoordinator ../pg*txt; touch mr-done ) &
sleep 1

# 启动多个可能崩溃的 worker
$TIMEOUT2 ../mrworker ../../mrapps/crash.so &

( while [ -e $SOCKNAME -a ! -f mr-done ]
  do
    $TIMEOUT2 ../mrworker ../../mrapps/crash.so
    sleep 1
  done ) &

( while [ -e $SOCKNAME -a ! -f mr-done ]
  do
    $TIMEOUT2 ../mrworker ../../mrapps/crash.so
    sleep 1
  done ) &

while [ -e $SOCKNAME -a ! -f mr-done ]
do
  $TIMEOUT2 ../mrworker ../../mrapps/crash.so
  sleep 1
done

wait

rm $SOCKNAME
sort mr-out* | grep . > mr-crash-all
if cmp mr-crash-all mr-correct-crash.txt
then
  echo '---' crash test: PASS
else
  echo '---' crash output is not the same as mr-correct-crash.txt
  echo '---' crash test: FAIL
fi
