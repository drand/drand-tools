#!/usr/bin/env bash

set -ex

if [[ $CI != "true" ]]; then
  stty -echoctl # hide ^C
fi

err() {
   echo "$@" > /dev/stderr
}

pushd "$(git rev-parse --show-toplevel)"

err "[+] Installing SIGINT trap ..."
bgPIDs=()

shutdown() {
    err "[+] Stopping instances ..."
    for pid in "${bgPIDs[@]}"; do
        if ps -p "${pid}" > /dev/null
        then
            kill -9 "${pid}"
        fi
    done

    exit 0
}

trap 'shutdown' SIGINT

tmp=$(mktemp -d)
err "[+] base tmp folder: $tmp"

err "[+] You can now use SIGINT to terminate this script ..."

if [ "$1" == "-w" ]; then
    wait=1
fi

if [ -z "${DRAND_SHARE_SECRET}" ]; then
    err "Please set DRAND_SHARE_SECRET"
    exit 1
fi

pushd $tmp

err "[+] fetching drand@v1.4.8 ..."
git clone --depth 1 --branch v1.4.8 https://github.com/drand/drand.git $tmp/drand-repo

pushd drand-repo

err "[+] installing drand dependencies ..."
go get -v ./...

err "[+] building drand ..."
go build -o $tmp/drand-test-v148

popd
rm -rf drand-repo
popd

f=("$tmp/d1" "$tmp/d2" "$tmp/d3")

p=(64100 64200 64300)

for dir in "${f[@]}"; do
    mkdir -p "$dir"
done

err "[+] generating keys"
for i in "${!f[@]}"; do
    $tmp/drand-test-v148 generate-keypair --tls-disable --folder "${f[${i}]}" 127.0.0.1:$((p["${i}"]+1))
done

err "[+] running drand daemons..."
for i in "${!f[@]}"; do
    $tmp/drand-test-v148 --verbose start --tls-disable --control "${p[${i}]}" --private-listen 127.0.0.1:$((p["${i}"]+1)) --metrics $((p["${i}"]+2)) --folder "${f[${i}]}" & # > $tmp/log1 2>&1 &
    bgPIDs+=("$!")
done

sleep 0.1

err "[+] Starting initial dkg ..."

if [ -n "$wait" ]; then
    err "About to start initial dkg, hit ENTER to start leader"
    read -r
fi

$tmp/drand-test-v148 share --control "${p[0]}" --tls-disable --leader --id default --period 1s --nodes ${#f[@]} --threshold ${#f[@]} &
bgPIDs+=("$!")

for i in $(seq 1 $((${#f[@]}-1))); do
    if [ -n "$wait" ]; then
        err "About to share node $i. Hit ENTER to continue"
        read -r
    fi

    $tmp/drand-test-v148 share --control "${p[${i}]}" --tls-disable --connect 127.0.0.1:$((p[0]+1))&
    bgPIDs+=("$!")
done

err "[+] Waiting 60s for the network to stabilize"
sleep 60

err "[+] done"

err "[+] building drand db-migration tool ..."
go build -o db-migration-test ./cmd/db-migration/

err "[+] checking database before migration..."
$tmp/drand-test-v148 sync --sync-nodes 127.0.0.1:$((p[0]+1)) --control "${p[0]}" --up-to 20

err "[+] Stopping instances ..."
for pid in "${bgPIDs[@]}"; do
    if ps -p "${pid}" > /dev/null
    then
        kill -9 "${pid}"
    fi
done

err "[+] Running db-migration tool ..."
./db-migration-test -beacon default -target bolt -source $tmp/d1/multibeacon/default/db/drand.db

pushd $tmp

err "[+] fetching drand@master ..."
git clone --depth 1 --branch master https://github.com/drand/drand.git $tmp/drand-repo

pushd drand-repo

err "[+] installing drand@master dependencies ..."
go get -v ./...

err "[+] building drand@master ..."
go build -o ../drand-master

popd
rm -rf drand-repo
popd

err "[+] running drand daemons..."
for i in "${!f[@]}"; do
    $tmp/drand-master --verbose start --tls-disable --control "${p[${i}]}" --private-listen 127.0.0.1:$((p["${i}"]+1)) --metrics $((p["${i}"]+2)) --folder "${f[${i}]}" & # > $tmp/log1 2>&1 &
    bgPIDs+=("$!")
done

sleep 1

err "[+] checking database after migration..."
$tmp/drand-master sync --sync-nodes 127.0.0.1:$((p[0]+1)) --control "${p[0]}" --up-to 20

sleep 1

err "[+] Waiting for SIGINT to terminate..."

for pid in "${bgPIDs[@]}"; do
    if ps -p "${pid}" > /dev/null
    then
        wait "${pid}"
    fi
done
