#!/bin/bash
if [[ -n $BAMBOO_DOCKER_AUTO_HOST ]]; then
sed -i "s/^.*Endpoint\": \"\(http:\/\/haproxy-ip-address:8000\)\".*$/    \"EndPoint\": \"http:\/\/$HOST:8000\",/" \
    ${CONFIG_PATH:=config/production.example.json}
fi
/usr/bin/supervisord
