for i in $(seq -f "%05g" 1 $1); do
    cat ~/go/azure-container-networking/test/scale/templates/kwok-node.yaml | sed "s/INSERT_NUMBER/$i/g" > "generated/kwok-nodes/node-$i.yaml"
done
