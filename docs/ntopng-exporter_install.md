# Guide to Installing ntopng-exporter

## Install ntopng on Ubuntu 22.04

```bash
root@ceph01:~# apt install ntopng -y

root@ceph01:~# nano /etc/ntopng.conf
----------------------- /etc/ntopng.conf --------------------------------------
# This configuration file is similar to the command line, with the exception
# that an equal sign '=' must be used between key and value. Example: -i=p1p2
# or --interface=p1p2 For options with no value (e.g. -v) the equal is also
# necessary. Example: "-v=" must be used.
#
# DO NOT REMOVE the following option, required for daemonization.
-e=

# * Interfaces to sniff on: one interface per line, prefix with -i=
# E.g.
#-i=eth0
#-i=wlan0
-i=ens18
# If none is specified, ntopng will try to auto-detect the best interface.
#
# * Port on which ntopng will listen for the web-UI.
-w=3001
----------------------- End of /etc/ntopng.conf -------------------------------

root@ceph01:~# sudo systemctl start ntopng
root@ceph01:~# sudo systemctl enable ntopng
root@ceph01:~# sudo systemctl status ntopng
● ntopng.service - ntopng - High-Speed Web-based Traffic Analysis and Flow Collection Tool
     Loaded: loaded (/lib/systemd/system/ntopng.service; enabled; vendor preset: enabled)
     Active: active (running) since Tue 2023-12-05 11:27:38 UTC; 4s ago
...
```

## Installing ntopng-exporter Attempt #2:

https://github.com/aauren/ntopng-exporter/releases/download/v1.2.1/ntopng-exporter_Linux_amd64.tar.gz

```bash
root@ceph01:~# wget https://github.com/aauren/ntopng-exporter/releases/download/v1.2.1/ntopng-exporter_Linux_amd64.tar.gz

root@ceph01:~# tar -xzvf ntopng-exporter_Linux_amd64.tar.gz
ntopng-exporter-1.2.1/LICENSE
ntopng-exporter-1.2.1/README.md
ntopng-exporter-1.2.1/config/ntopng-exporter.yaml
ntopng-exporter-1.2.1/resources/ntopng-exporter.service
ntopng-exporter-1.2.1/ntopng-exporter

root@ceph01:~# ls
crushmap.compiled  crushmap.temp  editable.map  ntopng-exporter-1.2.1  ntopng-exporter_Linux_amd64.tar.gz  snap

root@ceph01:~# adduser ntopng-exporter

root@ceph01:~# mkdir /home/ntopng-exporter/.ntopng-exporter
root@ceph01:~# chown ntopng-exporter:ntopng-exporter /home/ntopng-exporter/.ntopng-exporter
root@ceph01:~# cp ./ntopng-exporter-1.2.1/config/ntopng-exporter.yaml /home/ntopng-exporter/.ntopng-exporter/
root@ceph01:~# chmod 700 /home/ntopng-exporter/.ntopng-exporter/ntopng-exporter.yaml
root@ceph01:~# chown ntopng-exporter:ntopng-exporter /home/ntopng-exporter/.ntopng-exporter/ntopng-exporter.yaml

root@ceph01:~# nano /home/ntopng-exporter/.ntopng-exporter/ntopng-exporter.yaml
--------------------------------------- /home/ntopng-exporter/.ntopng-exporter/ntopng-exporter.yaml -------------------------------------------
ntopng:
  endpoint: "http://127.0.0.1:3001"
  allowUnsafeTLS: false # set to true to accept self-signed or otherwise unverifiable certs from ntopng (default: false)
  user: admin
  password: handbanana
  authMethod: cookie # cookie, basic, or none are accepted values
  scrapeInterval: 15s # scrape from the ntopng API every x period of time (should be synced with your prometheus scrapes) (default: 1 minute)
  scrapeTargets: # you can also specify "all" as a single list item to scrape all available endpoints (default: all)
  - hosts
  - interfaces
  - l7protocols

host:
  interfacesToMonitor:
  - ens18

metric:
  #localSubnetsOnly: # if this is defined, only include the local subnets defined here (greatly reduces number of metrics)
  #- "192.168.0.0/24"
  #- "224.0.0.0/4"
  excludeDNSMetrics: false # set to true, if you don't care about DNS metrics (also reduces number of metrics) (default: false)
  serve:
    ip: 0.0.0.0 # IP to serve metrics on, 0.0.0.0 is all interfaces (default: 0.0.0.0)
    port: 3002 # port to serve metrics on (default: 3001)
--------------------------------------- End of /home/ntopng-exporter/.ntopng-exporter/ntopng-exporter.yaml ------------------------------------
```

Copy the binary over:
```bash
root@ceph01:~# cp ./ntopng-exporter-1.2.1/ntopng-exporter /usr/local/bin/
root@ceph01:~# cp ./ntopng-exporter-1.2.1/config/ntopng-exporter.yaml /home/ntopng-exporter/.ntopng-exporter/
```

Copy the Systemd service file over:
```bash
root@ceph01:~# sudo cp ./ntopng-exporter-1.2.1/resources/ntopng-exporter.service /etc/systemd/system
root@ceph01:~# nano /etc/systemd/system/ntopng-exporter.service
----------------------- /etc/systemd/system/ntopng-exporter.service -----------------------------
[Unit]
Description=Exports Metrics for ntopng
After=ntopng.service
StartLimitIntervalSec=30
StartLimitBurst=5

[Service]
ExecStart=/usr/local/bin/ntopng-exporter
Restart=always
RestartSec=5
User=ntopng-exporter

[Install]
WantedBy=multi-user.target
--------------------- End of /etc/systemd/system/ntopng-exporter.service ------------------------
```
^ Note that we've added the depriviledged username here.

```bash
root@ceph01:~# sudo systemctl daemon-reload
root@ceph01:~# sudo systemctl enable ntopng-exporter
Created symlink /etc/systemd/system/multi-user.target.wants/ntopng-exporter.service → /etc/systemd/system/ntopng-exporter.service.
root@ceph01:~# sudo systemctl start ntopng-exporter
root@ceph01:~# sudo systemctl status ntopng-exporter
● ntopng-exporter.service - Exports Metrics for ntopng
     Loaded: loaded (/etc/systemd/system/ntopng-exporter.service; enabled; vendor preset: enabled)
     Active: active (running) since Tue 2023-12-05 11:32:10 UTC; 3s ago
   Main PID: 7794 (ntopng-exporter)
...
```

## Connecting this source to Prometheus
```bash

sudo nano /etc/prometheus/prometheus.yml
----------------------- /etc/prometheus/prometheus.yml -----------------------------
...
scrape_configs:
  - job_name: 'ntopng-exporter'
    # Override the global default and scrape targets from this job every 5 seconds.
    scrape_interval: 15s
    scrape_timeout: 15s
    static_configs:
      - targets: ['10.1.45.201:3002']
------------------ End of /etc/prometheus/prometheus.yml ---------------------------

sudo nano /etc/systemd/system/prometheus.service
----------------------- /etc/prometheus/prometheus.yml -----------------------------
[Unit]
Description=Prometheus Server
After=network-online.target

[Service]
User=prometheus
Restart=on-failure
ExecStart=/usr/bin/prometheus --config.file=/etc/prometheus/prometheus.yml --web.listen-address=":9150"

[Install]
WantedBy=multi-user.target
------------------- End of /etc/prometheus/prometheus.yml --------------------------

root@ceph01:~# sudo systemctl daemon-reload
root@ceph01:~# sudo systemctl start prometheus
root@ceph01:~# sudo systemctl enable prometheus
root@ceph01:~# sudo systemctl status prometheus
```

## Connecting it to Grafana!

Okay so I can't import dashboards into the Grafana that cephadm setup, so let's whip up another real quick.

```bash
sudo apt-get install -y software-properties-common
wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
sudo add-apt-repository "deb https://packages.grafana.com/oss/deb stable main"

sudo apt-get update
sudo apt-get install -y grafana

sudo nano /etc/grafana/grafana.ini
----------------------- /etc/grafana/grafana.ini -----------------------------
http_port = 3003
------------------- End of /etc/grafana/grafana.ini --------------------------

root@ceph01:~# sudo systemctl enable grafana-server
Synchronizing state of grafana-server.service with SysV service script with /lib/systemd/systemd-sysv-install.
Executing: /lib/systemd/systemd-sysv-install enable grafana-server
Created symlink /etc/systemd/system/multi-user.target.wants/grafana-server.service → /lib/systemd/system/grafana-server.service.
root@ceph01:~# sudo systemctl start grafana-server
root@ceph01:~# sudo systemctl status grafana-server
```

In the Grafana GUI, create a new Prometheus source with the Connection parameter: http://10.1.45.201:9150

Grafana reports 'No Data'... strange...


## Compiling a Custom ntopng-exporter
```bash
pcadmin@workstation:~/projects/ntopng-exporter$ go build ntopng-exporter.go
pcadmin@workstation:~/projects/ntopng-exporter$ ssh ceph01.snowsupport.top systemctl stop ntopng-exporter
pcadmin@workstation:~/projects/ntopng-exporter$ scp ./ntopng-exporter ceph01.snowsupport.top:/usr/local/bin/ntopng-exporter
ntopng-exporter                                                                                                                                                                                                                          100%   14MB  54.8MB/s   00:00
pcadmin@workstation:~/projects/ntopng-exporter$ ssh ceph01.snowsupport.top systemctl start ntopng-exporter
```
