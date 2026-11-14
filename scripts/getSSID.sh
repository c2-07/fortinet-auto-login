#!/bin/zsh

for i in ${(o)$(ifconfig -lX "en[0-9]")};
do 
    ipconfig getsummary ${i} | awk '/ SSID/ {print $NF}'
done 2> /dev/null
exit 0
