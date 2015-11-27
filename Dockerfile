FROM fpgeek/bamboo-base:latest

ADD . /opt/go/src/github.com/QubitProducts/bamboo
ADD builder/supervisord.conf.prod /etc/supervisor/conf.d/supervisord.conf
ADD builder/run.sh /run.sh
ADD builder/rsyslog.conf /etc/rsyslog.conf
ADD builder/haproxy_template.cfg /config/haproxy_template.cfg

WORKDIR /opt/go/src/github.com/QubitProducts/bamboo

RUN go get github.com/tools/godep && \
    go get -t github.com/smartystreets/goconvey && \
    go build && \
    ln -s /opt/go/src/github.com/QubitProducts/bamboo /var/bamboo && \
    mkdir -p /run/haproxy && \
    mkdir -p /var/log/supervisor && \
    echo "ENABLED=1" >> /etc/default/haproxy \
    echo "if (\$programname == 'haproxy') then -/var/log/haproxy.log" >> /etc/rsyslog.d/haproxy.conf

VOLUME "/var/log/supervisor"

RUN apt-get clean && \
    rm -rf /tmp/* /var/tmp/* && \
    rm -rf /var/lib/apt/lists/* && \
    rm -f /etc/dpkg/dpkg.cfg.d/02apt-speedup && \
    rm -f /etc/ssh/ssh_host_*

EXPOSE 80 8000

CMD /run.sh
