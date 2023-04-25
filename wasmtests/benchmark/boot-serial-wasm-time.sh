#num is the number of container
num=50
workdir=$(dirname "$(pwd)")
RAW=${workdir}/data/raw
TIME_DAT=${RAW}/boot-serial-wasm-time.dat
mkdir -p $workdir/json/container
mkdir -p $workdir/json/pod

#Function to get the interval time(ms)
function getTiming(){
    start=$1
    end=$2

    start_s=$(echo $start | cut -d '.' -f 1)
    start_ns=$(echo $start | cut -d '.' -f 2)
    end_s=$(echo $end | cut -d '.' -f 1)
    end_ns=$(echo $end | cut -d '.' -f 2)


    time=$(( ( 10#$end_s - 10#$start_s ) * 1000 + ( 10#$end_ns / 1000000 - 10#$start_ns / 1000000 ) ))

    echo "$time"
}

once_test(){
     i=$1
     #Create $PARALLEL container.json and pod.json
        cat > $workdir/json/container/container_$i.json << EOF
{
    "metadata": {
        "name": "testwasm",
        "namespace": "k8s.io"
    },
    "image": {
      "image": "wasi-demo-app",
      "annotations": {
          "wasm.module.url": "https://raw.githubusercontent.com/Youtirsin/wasi-demo-apps/main/wasi-demo-app.wasm",
          "wasm.module.filename": "wasi-demo-app.wasm"
          }
    },
    "command": [
        "wasi-demo-app.wasm", "daemon"
    ],
    "log_path":"wasm.log",
    "linux": {
    }
}
EOF

    cat > $workdir/json/pod/pod_$i.json <<EOF
{
    "metadata": {
        "name": "test-sandbox$i",
        "namespace": "k8s.io",
        "attempt": 1,
        "uid": "hdishd83djaidwnduwk28bcsb"
    },
    "log_directory": "/tmp",
    "linux": {
    }
}
EOF

#Start timing
start_time=$(date +%s.%N)

    # crictl run -r kuasar --no-pull $workdir/json/container/container_$i.json $workdir/json/pod/pod_$i.json 
    crictl -r unix:///run/containerd/containerd.sock run --runtime="wasm" --no-pull $workdir/json/container/container_$i.json $workdir/json/pod/pod_$i.json
    
#Wait for all the containers to finish starting
a=`crictl -r unix:///run/containerd/containerd.sock ps | grep testwasm | wc -l`
while [ $a -ne $(($i+1)) ];
do
a=`crictl -r unix:///run/containerd/containerd.sock ps | grep testwasm | wc -l`
done

#End timing
end_time=$(date +%s.%N)
boot_time=$(getTiming $start_time $end_time)

echo "BootTime: ${boot_time}ms"

#Output to the corresponding file
echo "${boot_time}" >> ${TIME_DAT}

}

#Kill all pods to prevent interference with testing
crictl -r unix:///run/containerd/containerd.sock rm -f -a
crictl -r unix:///run/containerd/containerd.sock rmp -f -a

for((i=0;i<$num;i++))
do
    once_test $i
    sleep 1s
done
