#!/bin/bash

rm -rf generated/kwok-nodes
mkdir -p generated/kwok-nodes
for j in $(seq 1 $1); do
    VAL1=$(( (j - 1) / 250 + 2 )) # 1-1000 -> 2-5
    VAL2=$(( (j - 1) % 250 )) # 1-1000 -> {1-250}
    i=$(printf "%05g" $j)
    #cat ~/go/azure-container-networking/test/scale/templates/kwok-node.yaml | sed -e "s/INSERT_NUMBER/$i/g" -e "s/VAL_1/$VAL1/g" -e "s/VAL_2/$VAL2/g" |> "generated/kwok-nodes/node-$i.yaml"
    fileName="/home/jshr/go/azure-container-networking/test/scale/templates/kwok-node.yaml"
    sed -e "s/VAL_1/$VAL1/g" -e "s/VAL_2/$VAL2/g" $fileName -e "s/INSERT_NUMBER/$i/g" > "generated/kwok-nodes/node-$i.yaml"
    #sed -i "s/VAL_2/$VAL2/g" $fileName
    #sed "s/INSERT_NUMBER/$i/g" $fileName > "generated/kwok-nodes/node-$i.yaml"
    #cat ~/go/azure-container-networking/test/scale/templates/kwok-node.yaml | sed -i "s/INSERT_NUMBER/$i/g" |> "generated/kwok-nodes/node-$i.yaml"
done
