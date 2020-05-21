#!/bin/bash

while [ true ]; do
  node index.js
  echo "Process exited $(echo $?) !! Waiting 5 seconds..."
  sleep 5
done