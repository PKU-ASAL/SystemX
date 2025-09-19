# SysArmor Wazuh API 实际测试结果


---

```
配置及测试方法:
wazuh初始配置见/shared/templates/configs/wazuh.yaml
首先配置环境变量export WAZUH_ENABLED=true 
然后正常运行manager启动 
可通过1.3 验证Wazuh配置 api 来更新自己的wazuh 配置，然后通过1.2来更新wazuh 配置，该api会自动更新wazuh.yaml并备份原始配置文件
即可测试下面的manager api.
```



## 1. 配置管理

### 1.1 获取Wazuh配置
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/config"
```

测试结果

```
{"data":{"manager":{"url":"https://10.129.81.4:55000","username":"wazuh","password":"Wf****CR","timeout":"","tls_verify":false,"status":"active"},"indexer":{"url":"https://10.129.81.4:9200","username":"admin","password":"5u****+D","timeout":"","tls_verify":false,"status":"active"},"status":"active"},"success":true}
```



### 1.2 更新Wazuh配置

```bash
curl -X PUT "http://localhost:8080/api/v1/wazuh/config" \
  -H "Content-Type: application/json" \
  -d '{
    "manager": {
      "url": "https://10.129.81.4:55000",
      "username": "wazuh",
      "password": "WfvmoiqFu*0g0t425lj*Y.3SBZOYmUCR"
    },
    "indexer": {
      "url": "https://10.129.81.4:9200",
      "username": "admin",
      "password": "5uSbeSyPANO?8rcbgvAF8frpANOWon+D"
    }
  }'
```

测试结果

```
{"message":"Configuration updated successfully","success":true}
```



### 1.3 验证Wazuh配置 

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/config/validate" \
  -H "Content-Type: application/json" \
  -d '{
    "manager": {
      "url": "https://10.129.81.4:55000",
      "username": "wazuh",
      "password": "WfvmoiqFu*0g0t425lj*Y.3SBZOYmUCR"
    },
    "indexer": {
      "url": "https://10.129.81.4:9200",
      "username": "admin",
      "password": "5uSbeSyPANO?8rcbgvAF8frpANOWon+D"
    }
  }'
```

测试结果

```
{"message":"Configuration is valid","success":true}
```

### 1.4 重新加载配置 

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/config/reload"
```

测试结果

```
{"message":"Configuration reloaded successfully","success":true}
```



---

## 2. Manager管理

### 2.1 获取Manager信息
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/info"
```

测试结果

```
{"data":{"data":{"affected_items":[{"path":"/var/ossec","version":"v4.12.0","type":"server","max_agents":"unlimited","openssl_support":"yes","tz_offset":"+0000","tz_name":"UTC"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"Basic information was successfully read","error":0},"success":true}
```



### 2.2 获取Manager状态

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/status"
```

```
{"data":{"data":{"affected_items":[{"wazuh-agentlessd":"stopped","wazuh-analysisd":"running","wazuh-authd":"running","wazuh-csyslogd":"running","wazuh-dbd":"stopped","wazuh-monitord":"running","wazuh-execd":"running","wazuh-integratord":"stopped","wazuh-logcollector":"running","wazuh-maild":"stopped","wazuh-remoted":"running","wazuh-reportd":"stopped","wazuh-syscheckd":"running","wazuh-clusterd":"stopped","wazuh-modulesd":"running","wazuh-db":"running","wazuh-apid":"running"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"Processes status was successfully read","error":0},"success":true}
```



### 2.3 获取Manager日志 

```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/logs"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/logs?limit=5&level=info&search=syscollector"
```

测试结果

```
{"data":{"data":{"affected_items":[{"description":" Evaluation finished.","level":"info","tag":"wazuh-modulesd:syscollector","timestamp":"2025-09-12T06:45:17Z"},{"description":" Starting evaluation.","level":"info","tag":"wazuh-modulesd:syscollector","timestamp":"2025-09-12T06:45:01Z"},{"description":" Evaluation finished.","level":"info","tag":"wazuh-modulesd:syscollector","timestamp":"2025-09-12T05:45:00Z"},{"description":" Starting evaluation.","level":"info","tag":"wazuh-modulesd:syscollector","timestamp":"2025-09-12T05:44:39Z"},{"description":" Evaluation finished.","level":"info","tag":"wazuh-modulesd:syscollector","timestamp":"2025-09-12T04:44:38Z"}],"failed_items":[],"total_affected_items":8,"total_failed_items":0},"error":0,"message":"Logs were successfully read"},"success":true}
```



### 2.4 获取Manager统计信息

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/stats"
```

测试结果

```
{"data":{"data":{"affected_items":[{"alerts":[{"level":0,"sigid":5521,"times":12},{"level":0,"sigid":5522,"times":12},{"level":0,"sigid":40700,"times":103},{"level":5,"sigid":40704,"times":2},{"level":0,"sigid":80700,"times":77},{"level":3,"sigid":80730,"times":273},{"level":2,"sigid":1002,"times":15},{"level":0,"sigid":530,"times":373},{"level":7,"sigid":531,"times":1},{"level":7,"sigid":533,"times":2},{"level":1,"sigid":535,"times":1},{"level":3,"sigid":502,"times":1},{"level":0,"sigid":515,"times":6},{"level":0,"sigid":221,"times":172},{"level":0,"sigid":19000,"times":4},{"level":0,"sigid":60103,"times":505},{"level":3,"sigid":60106,"times":60},{"level":0,"sigid":63100,"times":2},{"level":0,"sigid":61100,"times":3},{"level":3,"sigid":61104,"times":2},{"level":0,"sigid":60600,"times":3255},{"level":0,"sigid":60609,"times":8},{"level":3,"sigid":60610,"times":4},{"level":3,"sigid":60635,"times":4},{"level":0,"sigid":60640,"times":43},{"level":3,"sigid":60642,"times":6},{"level":3,"sigid":60668,"times":1},{"level":3,"sigid":60669,"times":1},{"level":0,"sigid":60795,"times":1},{"level":5,"sigid":60796,"times":1},{"level":3,"sigid":60798,"times":1},{"level":3,"sigid":60805,"times":1},{"level":5,"sigid":594,"times":12},{"level":5,"sigid":750,"times":13}],"events":7108,"firewall":0,"hour":4,"syscheck":31,"totalAlerts":4977},{"alerts":[{"level":0,"sigid":2830,"times":1},{"level":0,"sigid":5521,"times":2},{"level":0,"sigid":5522,"times":2},{"level":0,"sigid":40700,"times":250},{"level":0,"sigid":80700,"times":196},{"level":3,"sigid":80730,"times":1053},{"level":0,"sigid":530,"times":160},{"level":0,"sigid":221,"times":297},{"level":0,"sigid":60103,"times":84},{"level":3,"sigid":60106,"times":10},{"level":0,"sigid":61100,"times":1},{"level":0,"sigid":60600,"times":653},{"level":0,"sigid":60640,"times":24},{"level":3,"sigid":60642,"times":1}],"events":7270,"firewall":0,"hour":5,"syscheck":0,"totalAlerts":2734},{"alerts":[{"level":0,"sigid":2830,"times":1},{"level":0,"sigid":2900,"times":9},{"level":7,"sigid":2902,"times":2},{"level":7,"sigid":2904,"times":3},{"level":0,"sigid":5521,"times":4},{"level":0,"sigid":5522,"times":4},{"level":0,"sigid":40700,"times":293},{"level":0,"sigid":80700,"times":194},{"level":3,"sigid":80730,"times":1050},{"level":10,"sigid":23505,"times":3},{"level":7,"sigid":23504,"times":8},{"level":5,"sigid":23503,"times":5},{"level":3,"sigid":23502,"times":16},{"level":0,"sigid":530,"times":160},{"level":0,"sigid":221,"times":282},{"level":0,"sigid":60103,"times":76},{"level":3,"sigid":60106,"times":10},{"level":0,"sigid":60600,"times":652}],"events":7291,"firewall":0,"hour":6,"syscheck":0,"totalAlerts":2772}],"failed_items":[],"total_affected_items":3,"total_failed_items":0},"error":0,"message":"Statistical information for each node was successfully read"},"success":true}
```



### 2.5 重启Manager

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/manager/restart"
```

测试结果

```
{"message":"Manager restart initiated","success":true}
```



### 2.6 获取Manager配置

```bash
# 获取所有配置
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/configuration"

```

测试结果

```
{"data":{"data":{"affected_items":[{"alerts":{"email_alert_level":"12","log_alert_level":"3"},"auth":{"ciphers":"HIGH:!ADH:!EXP:!MD5:!RC4:!3DES:!CAMELLIA:@STRENGTH","disabled":"no","port":"1515","purge":"yes","ssl_auto_negotiate":"no","ssl_manager_cert":"etc/sslmanager.cert","ssl_manager_key":"etc/sslmanager.key","ssl_verify_host":"no","use_password":"no","use_source_ip":"no"},"cis-cat":{"ciscat_path":"wodles/ciscat","disabled":"yes","interval":"1d","java_path":"wodles/java","scan-on-start":"yes","timeout":"1800"},"cluster":{"bind_addr":"0.0.0.0","disabled":"yes","hidden":"no","name":"wazuh","node_name":"node01","node_type":"master","nodes":["NODE_IP"],"port":"1516"},"command":[{"executable":"disable-account","name":"disable-account","timeout_allowed":"yes"},{"executable":"restart-wazuh.exe","name":"restart-wazuh"},{"executable":"firewall-drop","name":"firewall-drop","timeout_allowed":"yes"},{"executable":"host-deny","name":"host-deny","timeout_allowed":"yes"},{"executable":"route-null","name":"route-null","timeout_allowed":"yes"},{"executable":"route-null.exe","name":"win_route-null","timeout_allowed":"yes"},{"executable":"netsh.exe","name":"netsh","timeout_allowed":"yes"}],"global":{"agents_disconnection_alert_time":"0","agents_disconnection_time":"10m","alerts_log":"yes","email_from":"wazuh@example.wazuh.com","email_log_source":"alerts.log","email_maxperhour":"12","email_notification":"no","email_to":"recipient@example.wazuh.com","jsonout_output":"yes","logall":"yes","logall_json":"yes","smtp_server":"smtp.example.wazuh.com","update_check":"yes","white_list":["127.0.0.1","^localhost.localdomain$","162.105.129.122"]},"indexer":{"enabled":"yes","hosts":["https://127.0.0.1:9200"],"ssl":{"certificate":["/etc/filebeat/certs/wazuh-server.pem"],"certificate_authorities":[{"ca":["/etc/filebeat/certs/root-ca.pem"]}],"key":["/etc/filebeat/certs/wazuh-server-key.pem"]}},"localfile":[{"command":"df -P","frequency":"360","log_format":"command"},{"alias":"netstat listening ports","command":"netstat -tulpn | sed 's/\\([[:alnum:]]\\+\\)\\ \\+[[:digit:]]\\+\\ \\+[[:digit:]]\\+\\ \\+\\(.*\\):\\([[:digit:]]*\\)\\ \\+\\([0-9\\.\\:\\*]\\+\\).\\+\\ \\([[:digit:]]*\\/[[:alnum:]\\-]*\\).*/\\1 \\2 == \\3 == \\4 \\5/' | sort -k 4 -g | sed 's/ == \\(.*\\) ==/:\\1/' | sed 1,2d","frequency":"360","log_format":"full_command"},{"command":"last -n 20","frequency":"360","log_format":"full_command"},{"location":"journald","log_format":"journald"},{"location":"/var/log/audit/audit.log","log_format":"audit"},{"location":"/var/ossec/logs/active-responses.log","log_format":"syslog"}],"osquery":{"add_labels":"yes","config_path":"/etc/osquery/osquery.conf","disabled":"yes","log_path":"/var/log/osquery/osqueryd.results.log","run_daemon":"yes"},"remote":[{"connection":"secure","port":"1514","protocol":["tcp"],"queue_size":"131072"}],"rootcheck":{"check_dev":"yes","check_files":"yes","check_if":"yes","check_pids":"yes","check_ports":"yes","check_sys":"yes","check_trojans":"yes","disabled":"no","frequency":"43200","ignore":"/var/lib/docker/overlay2","rootkit_files":["etc/rootcheck/rootkit_files.txt"],"rootkit_trojans":["etc/rootcheck/rootkit_trojans.txt"],"skip_nfs":"yes"},"ruleset":{"decoder_dir":["ruleset/decoders","etc/decoders"],"list":["etc/lists/audit-keys","etc/lists/amazon/aws-eventnames","etc/lists/security-eventchannel"],"rule_dir":["ruleset/rules","etc/rules"],"rule_exclude":["0215-policy_rules.xml"]},"sca":{"enabled":"yes","interval":"12h","scan_on_start":"yes","skip_nfs":"yes"},"syscheck":{"alert_new_files":"yes","auto_ignore":{"frequency":"10","item":"no","timeframe":"3600"},"directories":[{"path":"/etc"},{"path":"/usr/bin"},{"path":"/usr/sbin"},{"path":"/bin"},{"path":"/sbin"},{"path":"/boot"}],"disabled":"no","frequency":"43200","ignore":["/etc/mtab","/etc/hosts.deny","/etc/mail/statistics","/etc/random-seed","/etc/random.seed","/etc/adjtime","/etc/httpd/logs","/etc/utmpx","/etc/wtmpx","/etc/cups/certs","/etc/dumpdates","/etc/svc/volatile",{"item":".log$|.swp$","type":"sregex"}],"max_eps":"50","nodiff":["/etc/ssl/private.key"],"process_priority":"10","scan_on_start":"yes","skip_dev":"yes","skip_nfs":"yes","skip_proc":"yes","skip_sys":"yes","synchronization":{"enabled":"yes","interval":"5m","max_eps":"10"}},"syscollector":{"disabled":"no","hardware":"yes","interval":"1h","network":"yes","os":"yes","packages":"yes","ports":{"all":"no","item":"yes"},"processes":"yes","scan_on_start":"yes","synchronization":{"max_eps":["10"]}},"syslog_output":[{"format":"json","level":"0","port":"515","server":"10.7.96.207"}],"vulnerability-detection":{"enabled":"yes","feed-update-interval":"60m","index-status":"yes"}}],"failed_items":[],"total_affected_items":1,"total_failed_items":0},"error":0,"message":"Configuration was successfully read"},"success":true}
```




---

## 3. Agent管理

### 3.1 基础Agent管理 

#### 3.1.1 获取Agent列表 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents?limit=3&status=active&sort=name"

# 搜索特定Agent
curl -X GET "http://localhost:8080/api/v1/wazuh/agents?search=hfw"

# 按操作系统过滤
curl -X GET "http://localhost:8080/api/v1/wazuh/agents?status=active&sort=name&offset=0&limit=10"
```

测试结果示例

```
#带参数查询
{"data":{"data":{"affected_items":[{"id":"001","name":"hfw","ip":"10.7.108.98","register_ip":"","status":"active","status_code":0,"os":{"build":"19045.6332","major":"10","minor":"0","name":"Microsoft Windows 10 Pro","platform":"windows","uname":"Microsoft Windows 10 Pro","version":"10.0.19045.6332"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":["default","test-group-001","web-servers"],"config_sum":"","merged_sum":"","group_config_status":"synced"},{"id":"000","name":"master.instance.cloud.lcpu.dev","ip":"127.0.0.1","register_ip":"","status":"active","status_code":0,"os":{"major":"8","minor":"4","name":"CentOS Linux","platform":"centos","uname":"Linux |master.instance.cloud.lcpu.dev |4.18.0-305.3.1.el8.x86_64 |#1 SMP Tue Jun 1 16:14:33 UTC 2021 |x86_64","version":"8.4"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":null,"config_sum":"","merged_sum":"","group_config_status":"synced"},{"id":"010","name":"wdb","ip":"10.129.83.160","register_ip":"","status":"active","status_code":0,"os":{"major":"22","minor":"04","name":"Ubuntu","platform":"ubuntu","uname":"Linux |wdb |5.15.0-143-generic |#153-Ubuntu SMP Fri Jun 13 19:10:45 UTC 2025 |x86_64","version":"22.04.5 LTS"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":["default"],"config_sum":"","merged_sum":"","group_config_status":"synced"}],"total_affected_items":3,"total_failed_items":0,"failed_items":[]},"message":"All selected agents information was returned","error":0},"success":true}

#搜索特定agent
{"data":{"data":{"affected_items":[{"id":"001","name":"hfw","ip":"10.7.108.98","register_ip":"","status":"active","status_code":0,"os":{"build":"19045.6332","major":"10","minor":"0","name":"Microsoft Windows 10 Pro","platform":"windows","uname":"Microsoft Windows 10 Pro","version":"10.0.19045.6332"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":["default","test-group-001","web-servers"],"config_sum":"","merged_sum":"","group_config_status":"synced"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All selected agents information was returned","error":0},"success":true}
```



#### 3.1.2 添加新Agent 

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/agents" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-agent-007",
    "ip": "192.168.1.101"
  }'
```

测试结果

```
{"data":{"data":{"affected_items":null,"total_affected_items":0,"total_failed_items":0,"failed_items":null},"message":"","error":0},"success":true}
```



#### 3.1.3 获取单个Agent详情 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001"
```

测试结果

```
{"data":{"data":{"affected_items":[{"id":"001","name":"hfw","ip":"10.7.108.98","register_ip":"","status":"active","status_code":0,"os":{"build":"19045.6332","major":"10","minor":"0","name":"Microsoft Windows 10 Pro","platform":"windows","uname":"Microsoft Windows 10 Pro","version":"10.0.19045.6332"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":["default","test-group-001","web-servers"],"config_sum":"","merged_sum":"","group_config_status":"synced"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All selected agents information was returned","error":0},"success":true}
```



#### 3.1.4 删除Agent 

```bash
curl -X DELETE "http://localhost:8080/api/v1/wazuh/agents/018"
```

测试结果

```
{"message":"Agent deleted successfully","success":true}
```



#### 3.1.5 重启Agent 

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/agents/001/restart"
```

测试结果

```
{"message":"Agent restart initiated","success":true}
```



#### 3.1.6 获取Agent密钥 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/key"
```

测试结果

```
{"data":{"key":"MDAxIGhmdyBhbnkgNzM0MzQ2MGFmZjZjODJiMzBiODRiOThmODlmNGQ4ZWRlMjg4ZjFmNjg2NDM5NzQ2MjNkY2YyODZmNzhiNzIwYw=="},"success":true}
```



#### 3.1.7 升级Agent 

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/agents/001/upgrade" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4.12.0",
    "force": false
  }'
```

测试结果

```
{"message":"Agent upgrade initiated","success":true}
```



### 3.2 Agent详细信息 

#### 3.2.1 获取Agent系统信息 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/system"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","os":{"build":"19045.6332","display_version":"22H2","major":"10","minor":"0","name":"Microsoft Windows 10 Pro","version":"10.0.19045.6332"},"hostname":"DESKTOP-UOJJ5R1","architecture":"x86_64","os_release":"2009","scan":{"time":"2025-09-12T07:51:19+00:00"}}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



#### 3.2.2 获取Agent硬件信息 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/hardware"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","cpu":{"name":"Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz","cores":8,"mhz":1992},"ram":{"total":8230644,"free":4076892,"usage":50},"board_serial":"BBAJ04195Y002045"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



#### 3.2.3 获取Agent端口信息 

```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/ports"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/ports?limit=10&offset=0&sort=local.port&search=22"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","local":{"ip":"0.0.0.0","port":16422},"remote":{"ip":"0.0.0.0","port":0},"pid":5332,"protocol":"tcp","rx_queue":0,"tx_queue":0,"inode":0,"state":"listening","process":"QiyiService.exe"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



#### 3.2.4 获取Agent软件包信息 

```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/packages"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/packages?limit=5&search=nginx&sort=name"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","name":"7-Zip 24.08 (x64)","version":"24.08","vendor":"Igor Pavlov","description":" ","size":0,"architecture":"x86_64","format":"win","location":"C:\\Program Files\\7-Zip\\","priority":" ","section":" ","source":" ","install_time":"2025-04-14T01:56:33Z"},{"agent_id":"001","name":"AppGallery","version":"1.5.0.301","vendor":"Huawei Technologies Co., Ltd.","description":" ","size":0,"architecture":"x86_64","format":"win","location":" ","priority":" ","section":" ","source":" ","install_time":"2025-04-14T01:56:33Z"},{"agent_id":"001","name":"CAJViewer","version":"7.2.1","vendor":"TTKN","description":" ","size":0,"architecture":"i686","format":"win","location":" ","priority":" ","section":" ","source":" ","install_time":"2019-06-20T00:00:00Z"},{"agent_id":"001","name":"Clash for Windows 0.20.39","version":"0.20.39","vendor":"Fndroid","description":" ","size":0,"architecture":"x86_64","format":"win","location":" ","priority":" ","section":" ","source":" ","install_time":"2025-04-14T01:56:33Z"},{"agent_id":"001","name":"Cloud","version":"10.3.0.305","vendor":"Huawei Technologies Co., Ltd.","description":" ","size":0,"architecture":"x86_64","format":"win","location":" ","priority":" ","section":" ","source":" ","install_time":"2025-04-14T01:56:33Z"}],"total_affected_items":153,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



#### 3.2.5 获取Agent进程信息 

```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/processes"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/processes?limit=10&search=nginx&sort=pid"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","pid":"1004","name":"aesm_service.exe","cmd":"C:\\Windows\\System32\\DriverStore\\FileRepository\\sgx_psw.inf_amd64_71d5a06748fb983c\\aesm_service.exe","ppid":308,"utime":0,"stime":0,"start_time":1757470929,"vm_size":14692352,"size":2424832,"priority":8,"nlwp":2,"session":0},{"agent_id":"001","pid":"10300","name":"AggregatorHost.exe","cmd":"C:\\Windows\\System32\\AggregatorHost.exe","ppid":3296,"utime":14,"stime":12,"start_time":1757470812,"vm_size":20512768,"size":6860800,"priority":8,"nlwp":4,"session":0},{"agent_id":"001","pid":"10416","name":"AppVShNotify.exe","cmd":"C:\\Program Files\\Common Files\\microsoft shared\\ClickToRun\\AppVShNotify.exe","ppid":8668,"utime":0,"stime":0,"start_time":1757649217,"vm_size":10584064,"size":1765376,"priority":8,"nlwp":1,"session":0},{"agent_id":"001","pid":"10512","name":"svchost.exe","cmd":"C:\\Windows\\System32\\svchost.exe","ppid":308,"utime":0,"stime":1,"start_time":1757470818,"vm_size":34713600,"size":7000064,"priority":8,"nlwp":9,"session":0},{"agent_id":"001","pid":"1064","name":"svchost.exe","cmd":"C:\\Windows\\System32\\svchost.exe","ppid":308,"utime":21,"stime":10,"start_time":1757470798,"vm_size":33513472,"size":10870784,"priority":8,"nlwp":8,"session":0},{"agent_id":"001","pid":"10644","name":"SearchIndexer.exe","cmd":"C:\\Windows\\System32\\SearchIndexer.exe","ppid":308,"utime":0,"stime":0,"start_time":1757649258,"vm_size":49106944,"size":24776704,"priority":8,"nlwp":13,"session":0},{"agent_id":"001","pid":"1084","name":"IntelCpHDCPSvc.exe","cmd":"C:\\Windows\\System32\\DriverStore\\FileRepository\\iigd_dch.inf_amd64_4eaab4da752d9f3d\\IntelCpHDCPSvc.exe","ppid":308,"utime":0,"stime":0,"start_time":1757470799,"vm_size":9539584,"size":1531904,"priority":8,"nlwp":3,"session":0},{"agent_id":"001","pid":"1088","name":"fontdrvhost.exe","cmd":"C:\\Windows\\System32\\fontdrvhost.exe","ppid":904,"utime":0,"stime":0,"start_time":1757470798,"vm_size":10031104,"size":3043328,"priority":8,"nlwp":5,"session":0},{"agent_id":"001","pid":"10888","name":"svchost.exe","cmd":"C:\\Windows\\System32\\svchost.exe","ppid":308,"utime":0,"stime":0,"start_time":1757470930,"vm_size":25124864,"size":4689920,"priority":8,"nlwp":6,"session":0},{"agent_id":"001","pid":"1092","name":"fontdrvhost.exe","cmd":"C:\\Windows\\System32\\fontdrvhost.exe","ppid":992,"utime":0,"stime":0,"start_time":1757470798,"vm_size":7921664,"size":2236416,"priority":8,"nlwp":5,"session":1}],"total_affected_items":150,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



#### 3.2.6 获取Agent网络协议信息 

```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netproto"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netproto?limit=10&sort=iface"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","iface":"Loopback Pseudo-Interface 1","type":"ipv4","gateway":" ","dhcp":"disabled"},{"agent_id":"001","iface":"Loopback Pseudo-Interface 1","type":"ipv6","gateway":" ","dhcp":"disabled"},{"agent_id":"001","iface":"VMware Network Adapter VMnet1","type":"ipv6","gateway":" ","dhcp":"disabled"},{"agent_id":"001","iface":"VMware Network Adapter VMnet1","type":"ipv4","gateway":" ","dhcp":"disabled"},{"agent_id":"001","iface":"VMware Network Adapter VMnet8","type":"ipv6","gateway":" ","dhcp":"disabled"},{"agent_id":"001","iface":"VMware Network Adapter VMnet8","type":"ipv4","gateway":" ","dhcp":"disabled"},{"agent_id":"001","iface":"WLAN","type":"ipv4","gateway":"fe80::bed0:ebff:fe27:b802,10.7.0.1","dhcp":"enabled"},{"agent_id":"001","iface":"WLAN","type":"ipv6","gateway":"fe80::bed0:ebff:fe27:b802,10.7.0.1","dhcp":"enabled"},{"agent_id":"001","iface":"本地连接* 1","type":"ipv6","gateway":" ","dhcp":"enabled"},{"agent_id":"001","iface":"本地连接* 1","type":"ipv4","gateway":" ","dhcp":"enabled"}],"total_affected_items":14,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



#### 3.2.7 获取Agent网络地址信息 

```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netaddr"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netaddr?limit=10&search=eth0"
```

测试结果

```
{"data":{"data":{"affected_items":[{"agent_id":"001","iface":"Loopback Pseudo-Interface 1","address":"::1","netmask":"","proto":"ipv6","broadcast":" "},{"agent_id":"001","iface":"VMware Network Adapter VMnet8","address":"fe80::84dd:57c1:7254:449","netmask":"ffff:ffff:ffff:ffff::","proto":"ipv6","broadcast":" "},{"agent_id":"001","iface":"蓝牙网络连接","address":"169.254.31.148","netmask":"255.255.0.0","proto":"ipv4","broadcast":"169.254.255.255"},{"agent_id":"001","iface":"Loopback Pseudo-Interface 1","address":"127.0.0.1","netmask":"255.0.0.0","proto":"ipv4","broadcast":"127.255.255.255"},{"agent_id":"001","iface":"VMware Network Adapter VMnet1","address":"fe80::c510:f053:acf4:59fa","netmask":"ffff:ffff:ffff:ffff::","proto":"ipv6","broadcast":" "},{"agent_id":"001","iface":"VMware Network Adapter VMnet8","address":"192.168.168.1","netmask":"255.255.255.0","proto":"ipv4","broadcast":"192.168.168.255"},{"agent_id":"001","iface":"蓝牙网络连接","address":"fe80::2ede:7d2f:d519:6a5f","netmask":"ffff:ffff:ffff:ffff::","proto":"ipv6","broadcast":" "},{"agent_id":"001","iface":"VMware Network Adapter VMnet1","address":"192.168.216.1","netmask":"255.255.255.0","proto":"ipv4","broadcast":"192.168.216.255"},{"agent_id":"001","iface":"本地连接* 1","address":"fe80::e5b9:b379:f6f:507b","netmask":"ffff:ffff:ffff:ffff::","proto":"ipv6","broadcast":" "},{"agent_id":"001","iface":"WLAN","address":"10.7.108.98","netmask":"255.255.0.0","proto":"ipv4","broadcast":"10.7.255.255"}],"total_affected_items":18,"total_failed_items":0,"failed_items":[]},"message":"All specified syscollector information was returned","error":0},"success":true}
```



### 3.3 Agent统计信息 

#### 3.3.1 获取Agent日志收集器统计 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/stats/logcollector"
```

测试结果

```
{"data":{"data":{"affected_items":[{"global":{"start":"2025-09-12T15:51:18Z","end":"2025-09-12T16:14:18Z","files":[{"location":"active-response\\active-responses.log","events":0,"bytes":0,"targets":[{"name":"agent","drops":0}]},{"location":"System","events":0,"bytes":0,"targets":[{"name":"agent","drops":0}]},{"location":"Security","events":43,"bytes":76532,"targets":[{"name":"agent","drops":0}]},{"location":"Application","events":252,"bytes":149253,"targets":[{"name":"agent","drops":0}]}]},"interval":{"start":"2025-09-12T16:13:18Z","end":"2025-09-12T16:14:18Z","files":[{"location":"active-response\\active-responses.log","events":0,"bytes":0,"targets":[{"name":"agent","drops":0}]},{"location":"System","events":0,"bytes":0,"targets":[{"name":"agent","drops":0}]},{"location":"Security","events":0,"bytes":0,"targets":[{"name":"agent","drops":0}]},{"location":"Application","events":11,"bytes":6501,"targets":[{"name":"agent","drops":0}]}]}}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"Statistical information for each agent was successfully read","error":0},"success":true}
```



#### 3.3.2 获取Agent守护进程统计 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/daemons/stats"
```

测试结果

```
{"data":{"data":{"affected_items":[{"timestamp":"2025-09-12T08:15:01+00:00","name":"wazuh-remoted","agents":[{"id":1,"uptime":"2025-09-12T07:39:36+00:00","metrics":{"messages":{"received_breakdown":{"control":222,"control_breakdown":{"keepalive":214,"request":1,"shutdown":3,"startup":4},"event":2264},"sent_breakdown":{"ack":218,"ar":968,"discarded":0,"request":1,"sca":0,"shared":0}}}}]},{"timestamp":"2025-09-12T08:15:01+00:00","name":"wazuh-analysisd","agents":[{"id":1,"uptime":"2025-09-12T07:45:42+00:00","metrics":{"events":{"processed":385,"received_breakdown":{"decoded_breakdown":{"agent":2,"dbsync":1002,"integrations_breakdown":{"virustotal":0},"modules_breakdown":{"aws":0,"azure":0,"ciscat":0,"command":0,"docker":0,"gcp":0,"github":0,"logcollector_breakdown":{"eventchannel":374,"eventlog":0,"macos":0,"others":0},"ms-graph":0,"office365":0,"oscap":0,"osquery":0,"rootcheck":4,"sca":4,"syscheck":4,"syscollector":0,"upgrade":0,"vulnerability":0},"monitor":0,"remote":1}},"written_breakdown":{"alerts":7,"archives":385,"firewall":0}}}}]}],"total_affected_items":2,"total_failed_items":0,"failed_items":[]},"message":"Statistical information for each daemon was successfully read","error":0},"success":true}
```



### 3.4 Agent安全扫描 

#### 3.4.1 获取Agent CIS-CAT扫描结果 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/ciscat"
```

测试结果

```
{"data":{"data":{"affected_items":[],"total_affected_items":0,"total_failed_items":0,"failed_items":[]},"message":"No CISCAT results were returned","error":0},"success":true}
```

#### 3.4.2 获取Agent SCA扫描结果 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/sca"
```

测试结果

```
{"data":{"data":{"affected_items":[{"policy_id":"cis_win10_enterprise","name":"CIS Microsoft Windows 10 Enterprise Benchmark v1.12.0","description":"This document provides prescriptive guidance for establishing a secure configuration posture for Microsoft Windows 10 Enterprise.","references":"https://www.cisecurity.org/cis-benchmarks/","total_checks":394,"pass":116,"fail":274,"invalid":4,"score":29,"start_scan":"2025-09-12T07:51:23+00:00","end_scan":"2025-09-12T07:51:23+00:00","hash_file":"9edacb193e389710036f05af06265457f0018c88d0b5ab4bf4ccba6efb494317"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All selected SCA information was returned","error":0},"success":true}
```



#### 3.4.3 获取Agent Rootcheck扫描结果 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/rootcheck"
```

测试结果

```
{"data":{"data":{"affected_items":[],"total_affected_items":0,"total_failed_items":0,"failed_items":[]},"message":"No rootcheck information was returned","error":0},"success":true}
```



#### 3.4.4 清除Agent Rootcheck扫描结果 

```bash
curl -X DELETE "http://localhost:8080/api/v1/wazuh/agents/001/rootcheck"
```

#### 3.4.5 获取Agent Rootcheck最后扫描时间 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/rootcheck/last_scan"
```

测试结果

```
{"data":{"data":{"affected_items":[{"start":"2025-09-12T08:18:48Z","end":"2025-09-12T08:18:53Z"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"Last rootcheck scan of the agent was returned","error":0},"success":true}
```



### 3.5 Agent操作 

#### 3.5.1 执行Agent主动响应 
```bash
curl -X PUT "http://localhost:8080/api/v1/wazuh/agents/001/active-response" \
  -H "Content-Type: application/json" \
  -d '{
    "command": "restart-wazuh",
    "arguments": [],
    "alert": {
      "rule": {
        "id": 5715,
        "level": 5
      }
    }
  }'
```

#### 3.5.2 获取Agent升级结果 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/upgrade/result"
```



#### 3.5.3 批量升级Agents 

```bash
# 标准升级
curl -X PUT "http://localhost:8080/api/v1/wazuh/agents/upgrade" \
  -H "Content-Type: application/json" \
  -d '{
    "agents_list": ["001", "002", "003"],
    "upgrade_version": "4.12.0",
    "force": false
  }'

# 自定义升级
curl -X PUT "http://localhost:8080/api/v1/wazuh/agents/upgrade/custom" \
  -H "Content-Type: application/json" \
  -d '{
    "agents_list": ["001", "002"],
    "file_path": "/var/ossec/packages/wazuh-agent-4.12.0.deb"
  }'
```

测试结果

```
{"data":{"data":{"affected_items":[],"total_affected_items":0,"total_failed_items":1,"failed_items":[{"error":{"code":1822,"message":"Current agent version is greater or equal"},"id":["001"]}]},"message":"No upgrade task was created","error":1},"success":true}
```



#### 3.5.4 运行Rootcheck扫描 

```bash
curl -X PUT "http://localhost:8080/api/v1/wazuh/rootcheck" \
  -H "Content-Type: application/json" \
  -d '{
    "agents_list": ["001"]
  }'
```

测试结果

```
{"data":{"data":{"affected_items":["001"],"failed_items":[],"total_affected_items":1,"total_failed_items":0},"message":"Rootcheck scan was restarted on returned agents","error":0},"success":true}
```



### 3.6 集群和概览 

#### 3.7.1 获取集群健康状态 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/cluster/health"
```

测试结果

```
{"data":{"cluster_name":"wazuh-cluster","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":252,"active_shards":252,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":14,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":0,"active_shards_percent_as_number":94.73684210526315},"success":true}
```



#### 3.7.2 获取Agent概览信息 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/overview/agents"
```
测试结果

```
{"data":{"data":{"nodes":[{"node_name":"node01","count":4},{"node_name":"unknown","count":11}],"groups":[{"name":"1","count":1,"mergedSum":"fb872a6d1d0fa729ca7b65465d2a348a","configSum":"ab73af41699f13fdd81903b5f23d8d00"},{"name":"default","count":5,"mergedSum":"5af6936024d29d5aa77003f097fc3738","configSum":"19274f99ada0c0c117340a4482662413"},{"name":"test-group-001","count":1,"mergedSum":"7d3fa30858f7110fe811dc7df8b29328","configSum":"ab73af41699f13fdd81903b5f23d8d00"},{"name":"test-group-002","count":0,"mergedSum":"b328ec8a967640bf9ae9c3b0cd80e4b8","configSum":"ab73af41699f13fdd81903b5f23d8d00"},{"name":"wazuh-group-verification-test","count":2,"mergedSum":"7e2a4cc7f7515f61f4fff7ce5daf5d4d","configSum":"ab73af41699f13fdd81903b5f23d8d00"},{"name":"web-servers","count":1,"mergedSum":"15274e6d6972473a3b44894fdb1cca2a","configSum":"ab73af41699f13fdd81903b5f23d8d00"}],"agent_os":[{"os":{"name":"Microsoft Windows 10 Pro","platform":"windows","version":"10.0.19045.6332"},"count":1},{"os":{"name":"N/A","platform":"N/A","version":"N/A"},"count":11},{"os":{"name":"macOS","platform":"darwin","version":"15.5"},"count":2},{"os":{"name":"Ubuntu","platform":"ubuntu","version":"22.04.5 LTS"},"count":1}],"agent_status":{"connection":{"active":2,"disconnected":2,"never_connected":11,"pending":0,"total":15},"configuration":{"synced":4,"not_synced":11,"total":15}},"agent_version":[{"version":"Wazuh v4.12.0","count":4},{"version":"N/A","count":11}],"last_registered_agent":[{"id":"020","name":"test-agent-007","ip":"192.168.1.101","registerIP":"192.168.1.101","status":"never_connected","status_code":0,"node_name":"unknown","dateAdd":"2025-09-12T07:42:03+00:00","group_config_status":"not synced"}]},"error":0},"success":true}
```



------



## 4. 组管理

### 4.1 获取组列表
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/groups"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/groups?limit=5&sort=name&search=default"
```

测试结果

```
{"data":{"data":{"affected_items":[{"name":"1","count":1,"merged_sum":"","config_sum":""},{"name":"default","count":5,"merged_sum":"","config_sum":""},{"name":"test-group-001","count":1,"merged_sum":"","config_sum":""},{"name":"test-group-002","count":0,"merged_sum":"","config_sum":""},{"name":"wazuh-group-verification-test","count":2,"merged_sum":"","config_sum":""},{"name":"web-servers","count":1,"merged_sum":"","config_sum":""}],"total_affected_items":6,"total_failed_items":0,"failed_items":[]},"message":"All selected groups information was returned","error":0},"success":true}
```



### 4.2 创建新组

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/groups" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web-servers"
  }'
```

测试结果

```
{"message":"Group created successfully","success":true}
```



### 4.3 获取单个组信息

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/groups/default"
```

测试结果

```
{"data":{"data":{"affected_items":[{"name":"default","count":5,"merged_sum":"","config_sum":""}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"All selected groups information was returned","error":0},"success":true}
```



### 4.4 获取组内Agent列表

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/groups/default/agents"
```

测试结果

```
{"data":{"data":{"affected_items":[{"id":"001","name":"hfw","ip":"10.7.108.98","register_ip":"","status":"active","status_code":0,"os":{"build":"19045.6332","major":"10","minor":"0","name":"Microsoft Windows 10 Pro","platform":"windows","uname":"Microsoft Windows 10 Pro","version":"10.0.19045.6332"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":["default","test-group-001","web-servers"],"config_sum":"","merged_sum":"","group_config_status":"synced"},{"id":"003","name":"NewAgent","ip":"any","register_ip":"","status":"never_connected","status_code":0,"version":"","manager":"","node_name":"unknown","date_add":"0001-01-01T00:00:00Z","group":["1","default"],"config_sum":"","merged_sum":"","group_config_status":"not synced"},{"id":"008","name":"lxl-MacBook-Air.local","ip":"222.29.66.42","register_ip":"","status":"disconnected","status_code":4,"os":{"major":"15","minor":"5","name":"macOS","platform":"darwin","uname":"Darwin |lxl-MacBook-Air.local |24.5.0 |Darwin Kernel Version 24.5.0: Tue Apr 22 19:54:43 PDT 2025; root:xnu-11417.121.6~2/RELEASE_ARM64_T8132 |arm64","version":"15.5"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","disconnection_time":"2025-08-13T15:34:02Z","group":["default"],"config_sum":"","merged_sum":"","group_config_status":"synced"},{"id":"010","name":"wdb","ip":"10.129.83.160","register_ip":"","status":"active","status_code":0,"os":{"major":"22","minor":"04","name":"Ubuntu","platform":"ubuntu","uname":"Linux |wdb |5.15.0-143-generic |#153-Ubuntu SMP Fri Jun 13 19:10:45 UTC 2025 |x86_64","version":"22.04.5 LTS"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","group":["default"],"config_sum":"","merged_sum":"","group_config_status":"synced"},{"id":"011","name":"bogon","ip":"10.7.96.207","register_ip":"","status":"disconnected","status_code":4,"os":{"major":"15","minor":"5","name":"macOS","platform":"darwin","uname":"Darwin |lxl-MacBook-Air.local |24.5.0 |Darwin Kernel Version 24.5.0: Tue Apr 22 19:54:43 PDT 2025; root:xnu-11417.121.6~2/RELEASE_ARM64_T8132 |arm64","version":"15.5"},"version":"Wazuh v4.12.0","manager":"master.instance.cloud.lcpu.dev","node_name":"node01","date_add":"0001-01-01T00:00:00Z","disconnection_time":"2025-08-14T12:39:47Z","group":["default"],"config_sum":"","merged_sum":"","group_config_status":"synced"}],"total_affected_items":5,"total_failed_items":0,"failed_items":[]},"message":"All selected agents information was returned","error":0},"success":true}
```



### 4.5 添加Agent到组

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/groups/web-servers/agents" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "001"
  }'
```

测试结果

```
{"message":"Agent added to group successfully","success":true}
```



### 4.6 从组中移除Agent

```bash
curl -X DELETE "http://localhost:8080/api/v1/wazuh/groups/web-servers/agents/001"
```

测试结果

```
{"message":"Agent removed from group successfully","success":true}
```



### 4.7 获取组配置

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/groups/default/configuration"
```

测试结果

```
{"data":{"data":{"total_affected_items":1,"affected_items":[{"filters":{},"config":{"command":[{"executable":"ps aux","interval":"60","name":"check_running_processes"}],"localfile":[{"alias":"System Running Processes","command":"check_running_processes","log_format":"full_command"}]}}]},"error":0},"success":true}
```



---

## 5. 规则管理

### 5.1 获取规则列表
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/rules"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/rules?limit=10&search=ssh&sort=id"
```

测试结果

```
{"data":{"data":{"affected_items":[{"description":"SSHD messages grouped.","details":{"decoded_as":"sshd","noalert":"1"},"filename":"0095-sshd_rules.xml","gdpr":[],"gpg13":[],"groups":["syslog","sshd"],"hipaa":[],"id":5700,"level":0,"mitre":[],"nist_800_53":[],"pci_dss":[],"relative_dirname":"ruleset/rules","status":"enabled","tsc":[]},{"description":"sshd: Possible attack on the ssh server (or version gathering).","details":{"if_sid":"5700","match":{"pattern":"Bad protocol version identification"}},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d"],"gpg13":["4.12"],"groups":["recon","syslog","sshd"],"hipaa":[],"id":5701,"level":8,"mitre":["T1190"],"nist_800_53":["SI.4"],"pci_dss":["11.4"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]},{"description":"sshd: Reverse lookup error (bad ISP or attack).","details":{"if_sid":"5700","match":{"pattern":"^reverse mapping"},"regex":{"pattern":"failed - POSSIBLE BREAK"}},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d"],"gpg13":["4.12"],"groups":["syslog","sshd"],"hipaa":[],"id":5702,"level":5,"mitre":[],"nist_800_53":["SI.4"],"pci_dss":["11.4"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]},{"description":"sshd: Possible breakin attempt (high number of reverse lookup errors).","details":{"frequency":"6","if_matched_sid":"5702","same_source_ip":"","timeframe":"360"},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d"],"gpg13":["4.12"],"groups":["syslog","sshd"],"hipaa":[],"id":5703,"level":10,"mitre":["T1110"],"nist_800_53":["SI.4"],"pci_dss":["11.4"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]},{"description":"sshd: Timeout while logging in.","details":{"if_sid":"5700","match":{"pattern":"fatal: Timeout before authentication for"}},"filename":"0095-sshd_rules.xml","gdpr":[],"gpg13":[],"groups":["syslog","sshd"],"hipaa":[],"id":5704,"level":4,"mitre":[],"nist_800_53":[],"pci_dss":[],"relative_dirname":"ruleset/rules","status":"enabled","tsc":[]},{"description":"sshd: Possible scan or breakin attempt (high number of login timeouts).","details":{"frequency":"6","if_matched_sid":"5704","timeframe":"360"},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d"],"gpg13":["4.12"],"groups":["syslog","sshd"],"hipaa":[],"id":5705,"level":10,"mitre":["T1190","T1110"],"nist_800_53":["SI.4"],"pci_dss":["11.4"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]},{"description":"sshd: insecure connection attempt (scan).","details":{"if_sid":"5700","match":{"pattern":"Did not receive identification string from"}},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d"],"gpg13":["4.12"],"groups":["recon","syslog","sshd"],"hipaa":[],"id":5706,"level":6,"mitre":["T1021.004"],"nist_800_53":["SI.4"],"pci_dss":["11.4"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]},{"description":"sshd: OpenSSH challenge-response exploit.","details":{"if_sid":"5700","match":{"pattern":"fatal: buffer_get_string: bad string"}},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d"],"gpg13":["4.12"],"groups":["exploit_attempt","syslog","sshd"],"hipaa":[],"id":5707,"level":14,"mitre":["T1210","T1068"],"nist_800_53":["SI.4","SI.2"],"pci_dss":["11.4","6.2"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]},{"description":"sshd: Useless SSHD message without an user/ip and context.","details":{"if_sid":"5700","match":{"pattern":"error: Could not get shadow information for NOUSER|fatal: Read from socket failed: |error: ssh_msg_send: write|^syslogin_perform_logout: |^pam_succeed_if(sshd:auth): error retrieving information about user|can't verify hostname: getaddrinfo"}},"filename":"0095-sshd_rules.xml","gdpr":[],"gpg13":[],"groups":["syslog","sshd"],"hipaa":[],"id":5709,"level":0,"mitre":[],"nist_800_53":[],"pci_dss":[],"relative_dirname":"ruleset/rules","status":"enabled","tsc":[]},{"description":"sshd: Attempt to login using a non-existent user","details":{"if_sid":"5700","match":{"pattern":"illegal user|invalid user"}},"filename":"0095-sshd_rules.xml","gdpr":["IV_35.7.d","IV_32.2"],"gpg13":["7.1"],"groups":["authentication_failed","invalid_login","syslog","sshd"],"hipaa":["164.312.b"],"id":5710,"level":5,"mitre":["T1110.001","T1021.004"],"nist_800_53":["AU.14","AC.7","AU.6"],"pci_dss":["10.2.4","10.2.5","10.6.1"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.1","CC6.8","CC7.2","CC7.3"]}],"failed_items":[],"total_affected_items":93,"total_failed_items":0},"error":0,"message":"All selected rules were returned"},"success":true}
```



### 5.2 获取单个规则

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/rules/5715"
```

测试结果

```
{"data":{"data":{"affected_items":[{"description":"sshd: authentication success.","details":{"if_sid":"5700","match":{"pattern":"^Accepted|authenticated.$"}},"filename":"0095-sshd_rules.xml","gdpr":["IV_32.2"],"gpg13":["7.1","7.2"],"groups":["authentication_success","syslog","sshd"],"hipaa":["164.312.b"],"id":5715,"level":3,"mitre":["T1078","T1021"],"nist_800_53":["AU.14","AC.7"],"pci_dss":["10.2.5"],"relative_dirname":"ruleset/rules","status":"enabled","tsc":["CC6.8","CC7.2","CC7.3"]}],"failed_items":[],"total_affected_items":1,"total_failed_items":0},"error":0,"message":"All selected rules were returned"},"success":true}
```



## 6. Indexer管理

### 6.1 获取Indexer健康状态
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/health"
```

测试结果

```
{"data":{"cluster_name":"wazuh-cluster","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":252,"active_shards":252,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":14,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":0,"active_shards_percent_as_number":94.73684210526315},"success":true}
```



### 8.2 获取Indexer信息

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/info"
```

测试结果

```
{"data":{"cluster_name":"wazuh-cluster","cluster_uuid":"JzCS5kS7Rh6H7_A5vrotBQ","name":"node-1","tagline":"The OpenSearch Project: https://opensearch.org/","version":{"build_date":"2025-04-30T10:49:16.411257895Z","build_hash":"dae2bfc93896178873b43cdf4781f183c72b238f","build_snapshot":false,"build_type":"rpm","lucene_version":"9.12.1","minimum_index_compatibility_version":"7.0.0","minimum_wire_compatibility_version":"7.10.0","number":"7.10.2"}},"success":true}
```



### 8.3 获取索引列表

```bash
# 获取所有索引
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/indices"

# 获取特定模式的索引
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/indices?pattern=wazuh-alerts-*"
```

测试结果

```
{"data":{"indices":[{"index":"wazuh-alerts-4.x-2025.09.12","docs.count":"4348","store.size":"7.1mb","creation.date":"1757652349835"},{"index":"wazuh-alerts-4.x-2025.09.10","docs.count":"12662","store.size":"10.3mb","creation.date":"1757462405860"},{"index":"wazuh-alerts-4.x-2025.09.11","docs.count":"3876","store.size":"2.3mb","creation.date":"1757652339863"},{"index":"wazuh-alerts-4.x-2025.06.29","docs.count":"756","store.size":"2.4mb","creation.date":"1751162851280"},{"index":"wazuh-alerts-4.x-2025.06.28","docs.count":"1976","store.size":"3.6mb","creation.date":"1751068812358"},{"index":"wazuh-alerts-4.x-2025.06.25","docs.count":"918","store.size":"2.8mb","creation.date":"1750849666583"},{"index":"wazuh-alerts-4.x-2025.06.27","docs.count":"2855","store.size":"4.4mb","creation.date":"1751013931647"},{"index":"wazuh-alerts-4.x-2025.06.26","docs.count":"407","store.size":"1.7mb","creation.date":"1750896462347"},{"index":"wazuh-alerts-4.x-2025.07.01","docs.count":"8144","store.size":"8.2mb","creation.date":"1751328171764"},{"index":"wazuh-alerts-4.x-2025.08.14","docs.count":"3053","store.size":"4.7mb","creation.date":"1755162611088"},{"index":"wazuh-alerts-4.x-2025.07.22","docs.count":"31969","store.size":"11.5mb","creation.date":"1753142690867"},{"index":"wazuh-alerts-4.x-2025.08.31","docs.count":"25569","store.size":"15.3mb","creation.date":"1756632810023"},{"index":"wazuh-alerts-4.x-2025.07.20","docs.count":"411","store.size":"998.7kb","creation.date":"1752969678798"},{"index":"wazuh-alerts-4.x-2025.08.30","docs.count":"10306","store.size":"7.3mb","creation.date":"1756512047743"},{"index":"wazuh-alerts-4.x-2025.09.09","docs.count":"17264","store.size":"12.9mb","creation.date":"1757376006994"},{"index":"wazuh-alerts-4.x-2025.07.27","docs.count":"53756","store.size":"15.3mb","creation.date":"1753574410550"},{"index":"wazuh-alerts-4.x-2025.09.07","docs.count":"25736","store.size":"15.9mb","creation.date":"1757203231933"},{"index":"wazuh-alerts-4.x-2025.09.08","docs.count":"25598","store.size":"16.6mb","creation.date":"1757289612773"},{"index":"wazuh-alerts-4.x-2025.07.25","docs.count":"1124","store.size":"3.1mb","creation.date":"1753401894236"},{"index":"wazuh-alerts-4.x-2025.09.05","docs.count":"23537","store.size":"15.4mb","creation.date":"1757030413098"},{"index":"wazuh-alerts-4.x-2025.07.26","docs.count":"933","store.size":"2.5mb","creation.date":"1753488018451"},{"index":"wazuh-alerts-4.x-2025.09.06","docs.count":"25713","store.size":"17.1mb","creation.date":"1757116840019"}]},"success":true}
```



### 8.4获取索引模板

```bash
# 获取所有模板
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/templates"

# 获取特定模板
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/templates?pattern=wazuh"
```

测试结果

```
{"data":{"index_templates":[{"index_template":{"composed_of":[],"index_patterns":["wazuh-states-vulnerabilities-*"],"priority":1,"template":{"mappings":{"date_detection":false,"dynamic":"strict","properties":{"agent":{"properties":{"build":{"properties":{"original":{"ignore_above":1024,"type":"keyword"}}},"ephemeral_id":{"ignore_above":1024,"type":"keyword"},"id":{"ignore_above":1024,"type":"keyword"},"name":{"ignore_above":1024,"type":"keyword"},"type":{"ignore_above":1024,"type":"keyword"},"version":{"ignore_above":1024,"type":"keyword"}}},"host":{"properties":{"os":{"properties":{"full":{"ignore_above":1024,"type":"keyword"},"kernel":{"ignore_above":1024,"type":"keyword"},"name":{"ignore_above":1024,"type":"keyword"},"platform":{"ignore_above":1024,"type":"keyword"},"type":{"ignore_above":1024,"type":"keyword"},"version":{"ignore_above":1024,"type":"keyword"}}}}},"package":{"properties":{"architecture":{"ignore_above":1024,"type":"keyword"},"build_version":{"ignore_above":1024,"type":"keyword"},"checksum":{"ignore_above":1024,"type":"keyword"},"description":{"ignore_above":1024,"type":"keyword"},"install_scope":{"ignore_above":1024,"type":"keyword"},"installed":{"type":"date"},"license":{"ignore_above":1024,"type":"keyword"},"name":{"ignore_above":1024,"type":"keyword"},"path":{"ignore_above":1024,"type":"keyword"},"reference":{"ignore_above":1024,"type":"keyword"},"size":{"type":"long"},"type":{"ignore_above":1024,"type":"keyword"},"version":{"ignore_above":1024,"type":"keyword"}}},"vulnerability":{"properties":{"category":{"ignore_above":1024,"type":"keyword"},"classification":{"ignore_above":1024,"type":"keyword"},"description":{"ignore_above":1024,"type":"keyword"},"detected_at":{"type":"date"},"enumeration":{"ignore_above":1024,"type":"keyword"},"id":{"ignore_above":1024,"type":"keyword"},"published_at":{"type":"date"},"reference":{"ignore_above":1024,"type":"keyword"},"report_id":{"ignore_above":1024,"type":"keyword"},"scanner":{"properties":{"condition":{"ignore_above":1024,"type":"keyword"},"reference":{"ignore_above":1024,"type":"keyword"},"source":{"ignore_above":1024,"type":"keyword"},"vendor":{"ignore_above":1024,"type":"keyword"}}},"score":{"properties":{"base":{"type":"float"},"environmental":{"type":"float"},"temporal":{"type":"float"},"version":{"ignore_above":1024,"type":"keyword"}}},"severity":{"ignore_above":1024,"type":"keyword"},"under_evaluation":{"type":"boolean"}}},"wazuh":{"properties":{"cluster":{"properties":{"name":{"ignore_above":1024,"type":"keyword"},"node":{"ignore_above":1024,"type":"keyword"}}},"schema":{"properties":{"version":{"ignore_above":1024,"type":"keyword"}}}}}}},"settings":{"index":{"codec":"best_compression","mapping":{"total_fields":{"limit":"1000"}},"number_of_replicas":"0","number_of_shards":"1","query":{"default_field":["agent.id","host.os.full","host.os.version","package.name","package.version","vulnerability.id","vulnerability.description","vulnerability.severity","wazuh.cluster.name"]},"refresh_interval":"2s"}}}},"name":"wazuh-states-vulnerabilities-master.instance.cloud.lcpu.dev_template"},{"index_template":{"composed_of":[],"index_patterns":["wazuh-archives-*"],"priority":1,"template":{"mappings":{"dynamic_templates":[{"strings_as_keyword":{"mapping":{"type":"keyword"},"match_mapping_type":"string"}}],"properties":{"timestamp":{"format":"yyyy-MM-dd HH:mm:ss||yyyy-MM-dd HH:mm:ss.SSS||yyyy/MM/dd HH:mm:ss||yyyy/MM/dd HH:mm:ss.SSS||epoch_millis","type":"date"}}},"settings":{"index":{"number_of_replicas":"0","number_of_shards":"1","refresh_interval":"30s"}}}},"name":"wazuh-archives-template"}]},"success":true}
```




---

## 7. 告警查询

### 7.1 搜索告警
```bash
# 基础搜索
curl -X POST "http://localhost:8080/api/v1/wazuh/alerts/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": {
      "match_all": {}
    },
    "size": 10
  }'

# 高级搜索
curl -X POST "http://localhost:8080/api/v1/wazuh/alerts/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": {
      "bool": {
        "must": [
          {"range": {"rule.level": {"gte": 5}}},
          {"term": {"agent.name": "hfw"}}
        ]
      }
    },
    "size": 5,
    "sort": "@timestamp:desc"
  }'

```

测试结果

```
{"data":{"took":519,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"eq","value":5758},"max_score":0,"hits":[{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"I1YBPZkBNAigJEvDyhEH","_score":0,"_source":{"@timestamp":"2025-09-12T08:19:10.387Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"decoder":{"name":"syscheck_registry_value_modified"},"full_log":"Registry Value '[x32] HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits\\SecureTimeLow' modified\nMode: scheduled\nChanged attributes: md5,sha1,sha256\nOld md5sum was: '37df1399bcddd966d0956ce90eb7b7d2'\nNew md5sum is : '2d1e51dbeb52faa7dc98f91ef96de4fb'\nOld sha1sum was: '79cea29b60ca4db4bab899737f7b3beb84795e19'\nNew sha1sum is : '7159b8c480fe4be963968b022b81d0c32546e0aa'\nOld sha256sum was: '6747c93988a4f712a45258db63170e9264119fc4bc78aab734ce6381eb78c203'\nNew sha256sum is : 'e377f3d5fc7e41ad08ed5bda001c108a8085b7d322fdb6708e86e637aee509e7'\n","id":"1757665150.5516899","input":{"type":"log"},"location":"syscheck","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Registry Value Integrity Checksum Changed","firedtimes":9,"gdpr":["II_5.1.f"],"gpg13":["4.13"],"groups":["ossec","syscheck","syscheck_entry_modified","syscheck_registry"],"hipaa":["164.312.c.1","164.312.c.2"],"id":"750","level":5,"mail":false,"mitre":{"id":["T1565.001","T1112"],"tactic":["Impact","Defense Evasion"],"technique":["Stored Data Manipulation","Modify Registry"]},"nist_800_53":["SI.7"],"pci_dss":["11.5"],"tsc":["PI1.4","PI1.5","CC6.1","CC6.8","CC7.2","CC7.3"]},"syscheck":{"arch":"[x32]","changed_attributes":["md5","sha1","sha256"],"event":"modified","md5_after":"2d1e51dbeb52faa7dc98f91ef96de4fb","md5_before":"37df1399bcddd966d0956ce90eb7b7d2","mode":"scheduled","path":"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits","sha1_after":"7159b8c480fe4be963968b022b81d0c32546e0aa","sha1_before":"79cea29b60ca4db4bab899737f7b3beb84795e19","sha256_after":"e377f3d5fc7e41ad08ed5bda001c108a8085b7d322fdb6708e86e637aee509e7","sha256_before":"6747c93988a4f712a45258db63170e9264119fc4bc78aab734ce6381eb78c203","size_after":"8","value_name":"SecureTimeLow","value_type":"REG_QWORD"},"timestamp":"2025-09-12T08:19:10.387+0000"}},{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"IlYBPZkBNAigJEvDyhEH","_score":0,"_source":{"@timestamp":"2025-09-12T08:19:10.366Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"decoder":{"name":"syscheck_registry_value_modified"},"full_log":"Registry Value '[x32] HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits\\SecureTimeHigh' modified\nMode: scheduled\nChanged attributes: md5,sha1,sha256\nOld md5sum was: '8d8ca2699b142866db9aec3a34e21cf0'\nNew md5sum is : '7311357ee8099cf9e3d91fc86cdd2dcc'\nOld sha1sum was: 'c7f3066dc69f0f870be35c3648dc74b798bb3d1e'\nNew sha1sum is : '843204c626d9934a5275389c02f6c60b541817d1'\nOld sha256sum was: '81fee2f39ab2b2a261758daf1a91cd4e94889a8123e17e155e23d30ea21ffd7d'\nNew sha256sum is : 'dd390672fe28203e23f0ee1e722414821ff06a3d8af0d0b32a512c397c024fa3'\n","id":"1757665150.5515788","input":{"type":"log"},"location":"syscheck","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Registry Value Integrity Checksum Changed","firedtimes":8,"gdpr":["II_5.1.f"],"gpg13":["4.13"],"groups":["ossec","syscheck","syscheck_entry_modified","syscheck_registry"],"hipaa":["164.312.c.1","164.312.c.2"],"id":"750","level":5,"mail":false,"mitre":{"id":["T1565.001","T1112"],"tactic":["Impact","Defense Evasion"],"technique":["Stored Data Manipulation","Modify Registry"]},"nist_800_53":["SI.7"],"pci_dss":["11.5"],"tsc":["PI1.4","PI1.5","CC6.1","CC6.8","CC7.2","CC7.3"]},"syscheck":{"arch":"[x32]","changed_attributes":["md5","sha1","sha256"],"event":"modified","md5_after":"7311357ee8099cf9e3d91fc86cdd2dcc","md5_before":"8d8ca2699b142866db9aec3a34e21cf0","mode":"scheduled","path":"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits","sha1_after":"843204c626d9934a5275389c02f6c60b541817d1","sha1_before":"c7f3066dc69f0f870be35c3648dc74b798bb3d1e","sha256_after":"dd390672fe28203e23f0ee1e722414821ff06a3d8af0d0b32a512c397c024fa3","sha256_before":"81fee2f39ab2b2a261758daf1a91cd4e94889a8123e17e155e23d30ea21ffd7d","size_after":"8","value_name":"SecureTimeHigh","value_type":"REG_QWORD"},"timestamp":"2025-09-12T08:19:10.366+0000"}},{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"IVYBPZkBNAigJEvDyhEH","_score":0,"_source":{"@timestamp":"2025-09-12T08:19:10.351Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"decoder":{"name":"syscheck_registry_value_modified"},"full_log":"Registry Value '[x32] HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits\\SecureTimeEstimated' modified\nMode: scheduled\nChanged attributes: md5,sha1,sha256\nOld md5sum was: '35fe26e31818c6189d56a7e605d9bb49'\nNew md5sum is : '6d8952604ca724d9a35c8ab947203140'\nOld sha1sum was: '24239c272d41d44f7e1631e4bf97f74852b5e9a9'\nNew sha1sum is : 'e788e69f88d76094472906e4cb3b6dc7134cd50d'\nOld sha256sum was: '38fd8b9b645ce135564064fffbf9e9f6cfaf480117d59665770b2e4b8a009885'\nNew sha256sum is : '7c31ea71378e9a04f3994e72f0c7beab58388513464f2a4263c27cc095a76ed0'\n","id":"1757665150.5514672","input":{"type":"log"},"location":"syscheck","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Registry Value Integrity Checksum Changed","firedtimes":7,"gdpr":["II_5.1.f"],"gpg13":["4.13"],"groups":["ossec","syscheck","syscheck_entry_modified","syscheck_registry"],"hipaa":["164.312.c.1","164.312.c.2"],"id":"750","level":5,"mail":false,"mitre":{"id":["T1565.001","T1112"],"tactic":["Impact","Defense Evasion"],"technique":["Stored Data Manipulation","Modify Registry"]},"nist_800_53":["SI.7"],"pci_dss":["11.5"],"tsc":["PI1.4","PI1.5","CC6.1","CC6.8","CC7.2","CC7.3"]},"syscheck":{"arch":"[x32]","changed_attributes":["md5","sha1","sha256"],"event":"modified","md5_after":"6d8952604ca724d9a35c8ab947203140","md5_before":"35fe26e31818c6189d56a7e605d9bb49","mode":"scheduled","path":"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits","sha1_after":"e788e69f88d76094472906e4cb3b6dc7134cd50d","sha1_before":"24239c272d41d44f7e1631e4bf97f74852b5e9a9","sha256_after":"7c31ea71378e9a04f3994e72f0c7beab58388513464f2a4263c27cc095a76ed0","sha256_before":"38fd8b9b645ce135564064fffbf9e9f6cfaf480117d59665770b2e4b8a009885","size_after":"8","value_name":"SecureTimeEstimated","value_type":"REG_QWORD"},"timestamp":"2025-09-12T08:19:10.351+0000"}},{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"IFYBPZkBNAigJEvDyhEH","_score":0,"_source":{"@timestamp":"2025-09-12T08:19:10.331Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"decoder":{"name":"syscheck_registry_key_modified"},"full_log":"Registry Key '[x32] HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits' modified\nMode: scheduled\nChanged attributes: mtime\nOld modification time was: '1757663417', now it is '1757664917'\n","id":"1757665150.5513275","input":{"type":"log"},"location":"syscheck","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Registry Key Integrity Checksum Changed","firedtimes":3,"gdpr":["II_5.1.f"],"gpg13":["4.13"],"groups":["ossec","syscheck","syscheck_entry_modified","syscheck_registry"],"hipaa":["164.312.c.1","164.312.c.2"],"id":"594","level":5,"mail":false,"mitre":{"id":["T1565.001","T1112"],"tactic":["Impact","Defense Evasion"],"technique":["Stored Data Manipulation","Modify Registry"]},"nist_800_53":["SI.7"],"pci_dss":["11.5"],"tsc":["PI1.4","PI1.5","CC6.1","CC6.8","CC7.2","CC7.3"]},"syscheck":{"arch":"[x32]","changed_attributes":["mtime"],"event":"modified","gid_after":"S-1-5-18","gname_after":"SYSTEM","mode":"scheduled","mtime_after":"2025-09-12T08:15:17","mtime_before":"2025-09-12T07:50:17","path":"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits","uid_after":"S-1-5-18","uname_after":"SYSTEM","win_perm_after":[{"allowed":["GENERIC_READ","READ_CONTROL","READ_DATA","READ_EA","WRITE_EA"],"name":"Users"},{"allowed":["GENERIC_ALL","DELETE","READ_CONTROL","WRITE_DAC","WRITE_OWNER","READ_DATA","WRITE_DATA","APPEND_DATA","READ_EA","WRITE_EA","EXECUTE"],"name":"Administrators"},{"allowed":["GENERIC_ALL","DELETE","READ_CONTROL","WRITE_DAC","WRITE_OWNER","READ_DATA","WRITE_DATA","APPEND_DATA","READ_EA","WRITE_EA","EXECUTE"],"name":"SYSTEM"},{"allowed":["GENERIC_READ","READ_CONTROL","READ_DATA","READ_EA","WRITE_EA"],"name":"NETWORK SERVICE"},{"allowed":["GENERIC_READ","READ_CONTROL","READ_DATA","READ_EA","WRITE_EA"],"name":"LOCAL SERVICE"},{"allowed":["GENERIC_READ","READ_CONTROL","READ_DATA","READ_EA","WRITE_EA"],"name":"Network Configuration Operators"},{"allowed":["GENERIC_READ","GENERIC_WRITE","DELETE","READ_CONTROL","READ_DATA","WRITE_DATA","APPEND_DATA","READ_EA","WRITE_EA"],"name":"autotimesvc"}]},"timestamp":"2025-09-12T08:19:10.331+0000"}},{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"H1YBPZkBNAigJEvDyhEH","_score":0,"_source":{"@timestamp":"2025-09-12T08:19:10.328Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"decoder":{"name":"syscheck_registry_value_modified"},"full_log":"Registry Value '[x32] HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits\\RunTime\\SecureTimeTickCount' modified\nMode: scheduled\nChanged attributes: md5,sha1,sha256\nOld md5sum was: '95ceae89c820a663580cb084359b8d45'\nNew md5sum is : 'db953924af1ec6ea1c87a221a75ac43d'\nOld sha1sum was: '9493102b0ee4251cbfa17794c942e46dba70f153'\nNew sha1sum is : '28c4030883d99a6af6bc7808c069cc9cc9457ee4'\nOld sha256sum was: 'df6c46a018e73cf085b0ce57658996c7cd5ca88b06f2a34fc0c9a42941307343'\nNew sha256sum is : '8d59ee04f0781b48303f0dd7babfc3a24a5649ebbedb14c4fc0aa4f0c95093d8'\n","id":"1757665150.5512151","input":{"type":"log"},"location":"syscheck","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Registry Value Integrity Checksum Changed","firedtimes":6,"gdpr":["II_5.1.f"],"gpg13":["4.13"],"groups":["ossec","syscheck","syscheck_entry_modified","syscheck_registry"],"hipaa":["164.312.c.1","164.312.c.2"],"id":"750","level":5,"mail":false,"mitre":{"id":["T1565.001","T1112"],"tactic":["Impact","Defense Evasion"],"technique":["Stored Data Manipulation","Modify Registry"]},"nist_800_53":["SI.7"],"pci_dss":["11.5"],"tsc":["PI1.4","PI1.5","CC6.1","CC6.8","CC7.2","CC7.3"]},"syscheck":{"arch":"[x32]","changed_attributes":["md5","sha1","sha256"],"event":"modified","md5_after":"db953924af1ec6ea1c87a221a75ac43d","md5_before":"95ceae89c820a663580cb084359b8d45","mode":"scheduled","path":"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits\\RunTime","sha1_after":"28c4030883d99a6af6bc7808c069cc9cc9457ee4","sha1_before":"9493102b0ee4251cbfa17794c942e46dba70f153","sha256_after":"8d59ee04f0781b48303f0dd7babfc3a24a5649ebbedb14c4fc0aa4f0c95093d8","sha256_before":"df6c46a018e73cf085b0ce57658996c7cd5ca88b06f2a34fc0c9a42941307343","size_after":"8","value_name":"SecureTimeTickCount","value_type":"REG_QWORD"},"timestamp":"2025-09-12T08:19:10.328+0000"}}]}},"success":true
```



### 7.2 根据Agent获取告警 

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/agent/001?limit=2"
```

测试结果

```
{"data":{"took":55,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"b1YWPZkBNAigJEvDRx3e","_score":0,"_source":{"@timestamp":"2025-09-12T08:41:31.658Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"data":{"win":{"eventdata":{"authenticationPackageName":"Negotiate","elevatedToken":"%%1842","impersonationLevel":"%%1833","keyLength":"0","logonGuid":"{00000000-0000-0000-0000-000000000000}","logonProcessName":"Advapi","logonType":"5","processId":"0x134","processName":"C:\\\\Windows\\\\System32\\\\services.exe","subjectDomainName":"WORKGROUP","subjectLogonId":"0x3e7","subjectUserName":"DESKTOP-UOJJ5R1$","subjectUserSid":"S-1-5-18","targetDomainName":"NT AUTHORITY","targetLinkedLogonId":"0x0","targetLogonId":"0x3e7","targetUserName":"SYSTEM","targetUserSid":"S-1-5-18","virtualAccount":"%%1843"},"system":{"channel":"Security","computer":"DESKTOP-UOJJ5R1","eventID":"4624","eventRecordID":"279007","keywords":"0x8020000000000000","level":"0","message":"\"已成功登录帐户。\r\n\r\n使用者:\r\n\t安全 ID:\t\tS-1-5-18\r\n\t帐户名称:\t\tDESKTOP-UOJJ5R1$\r\n\t帐户域:\t\tWORKGROUP\r\n\t登录 ID:\t\t0x3E7\r\n\r\n登录信息:\r\n\t登录类型:\t\t5\r\n\t受限制的管理员模式:\t-\r\n\t虚拟帐户:\t\t否\r\n\t提升的令牌:\t\t是\r\n\r\n模拟级别:\t\t模拟\r\n\r\n新登录:\r\n\t安全 ID:\t\tS-1-5-18\r\n\t帐户名称:\t\tSYSTEM\r\n\t帐户域:\t\tNT AUTHORITY\r\n\t登录 ID:\t\t0x3E7\r\n\t链接的登录 ID:\t\t0x0\r\n\t网络帐户名称:\t-\r\n\t网络帐户域:\t-\r\n\t登录 GUID:\t\t{00000000-0000-0000-0000-000000000000}\r\n\r\n进程信息:\r\n\t进程 ID:\t\t0x134\r\n\t进程名称:\t\tC:\\Windows\\System32\\services.exe\r\n\r\n网络信息:\r\n\t工作站名称:\t-\r\n\t源网络地址:\t-\r\n\t源端口:\t\t-\r\n\r\n详细的身份验证信息:\r\n\t登录进程:\t\tAdvapi  \r\n\t身份验证数据包:\tNegotiate\r\n\t传递的服务:\t-\r\n\t数据包名(仅限 NTLM):\t-\r\n\t密钥长度:\t\t0\r\n\r\n创建登录会话时，将在被访问的计算机上生成此事件。\r\n\r\n“使用者”字段指示本地系统上请求登录的帐户。这通常是一个服务(例如 Server 服务)或本地进程(例如 Winlogon.exe 或 Services.exe)。\r\n\r\n“登录类型”字段指示发生的登录类型。最常见的类型是 2 (交互式)和 3 (网络)。\r\n\r\n“新登录”字段指示新登录是为哪个帐户创建的，即已登录的帐户。\r\n\r\n“网络”字段指示远程登录请求源自哪里。“工作站名称”并非始终可用，并且在某些情况下可能会留空。\r\n\r\n“模拟级别”字段指示登录会话中的进程可以模拟到的程度。\r\n\r\n“身份验证信息”字段提供有关此特定登录请求的详细信息。\r\n\t- “登录 GUID”是可用于将此事件与 KDC 事件关联起来的唯一标识符。\r\n\t-“传递的服务”指示哪些中间服务参与了此登录请求。\r\n\t-“数据包名”指示在 NTLM 协议中使用了哪些子协议。\r\n\t-“密钥长度”指示生成的会话密钥的长度。如果没有请求会话密钥，则此字段将为 0。\"","opcode":"0","processID":"504","providerGuid":"{54849625-5478-4994-a5ba-3e3b0328c30d}","providerName":"Microsoft-Windows-Security-Auditing","severityValue":"AUDIT_SUCCESS","systemTime":"2025-09-12T08:41:30.6774170Z","task":"12544","threadID":"3532","version":"2"}}},"decoder":{"name":"windows_eventchannel"},"id":"1757666491.6033980","input":{"type":"log"},"location":"EventChannel","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Windows Logon Success","firedtimes":8,"gdpr":["IV_32.2"],"gpg13":["7.1","7.2"],"groups":["windows","windows_security","authentication_success"],"hipaa":["164.312.b"],"id":"60106","level":3,"mail":false,"mitre":{"id":["T1078"],"tactic":["Defense Evasion","Persistence","Privilege Escalation","Initial Access"],"technique":["Valid Accounts"]},"nist_800_53":["AU.14","AC.9"],"pci_dss":["10.2.5"],"tsc":["CC6.8","CC7.2","CC7.3"]},"timestamp":"2025-09-12T08:41:31.658+0000"}},{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"iFYUPZkBNAigJEvDrRyY","_score":0,"_source":{"@timestamp":"2025-09-12T08:39:43.427Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"data":{"win":{"eventdata":{"data":"2025-09-16T06:41:43Z, RulesEngine"},"system":{"channel":"Application","computer":"DESKTOP-UOJJ5R1","eventID":"16384","eventRecordID":"45075","eventSourceName":"Software Protection Platform Service","keywords":"0x80000000000000","level":"4","message":"\"安排软件保护服务在 2025-09-16T06:41:43Z 时重新启动成功。原因: RulesEngine。\"","opcode":"0","processID":"0","providerGuid":"{E23B33B0-C8C9-472C-A5F9-F2BDFEA0F156}","providerName":"Microsoft-Windows-Security-SPP","severityValue":"INFORMATION","systemTime":"2025-09-12T08:39:43.4396358Z","task":"0","threadID":"0","version":"0"}}},"decoder":{"name":"windows_eventchannel"},"id":"1757666383.5992471","input":{"type":"log"},"location":"EventChannel","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Software protection service scheduled successfully.","firedtimes":2,"groups":["windows","windows_application"],"id":"60642","level":3,"mail":false},"timestamp":"2025-09-12T08:39:43.427+0000"}}]}},"success":true}
```



### 7.3 根据规则获取告警

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/rule/60106?limit=2"
```

测试结果

```
{"data":{"took":72,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"b1YWPZkBNAigJEvDRx3e","_score":0,"_source":{"@timestamp":"2025-09-12T08:41:31.658Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"data":{"win":{"eventdata":{"authenticationPackageName":"Negotiate","elevatedToken":"%%1842","impersonationLevel":"%%1833","keyLength":"0","logonGuid":"{00000000-0000-0000-0000-000000000000}","logonProcessName":"Advapi","logonType":"5","processId":"0x134","processName":"C:\\\\Windows\\\\System32\\\\services.exe","subjectDomainName":"WORKGROUP","subjectLogonId":"0x3e7","subjectUserName":"DESKTOP-UOJJ5R1$","subjectUserSid":"S-1-5-18","targetDomainName":"NT AUTHORITY","targetLinkedLogonId":"0x0","targetLogonId":"0x3e7","targetUserName":"SYSTEM","targetUserSid":"S-1-5-18","virtualAccount":"%%1843"},"system":{"channel":"Security","computer":"DESKTOP-UOJJ5R1","eventID":"4624","eventRecordID":"279007","keywords":"0x8020000000000000","level":"0","message":"\"已成功登录帐户。\r\n\r\n使用者:\r\n\t安全 ID:\t\tS-1-5-18\r\n\t帐户名称:\t\tDESKTOP-UOJJ5R1$\r\n\t帐户域:\t\tWORKGROUP\r\n\t登录 ID:\t\t0x3E7\r\n\r\n登录信息:\r\n\t登录类型:\t\t5\r\n\t受限制的管理员模式:\t-\r\n\t虚拟帐户:\t\t否\r\n\t提升的令牌:\t\t是\r\n\r\n模拟级别:\t\t模拟\r\n\r\n新登录:\r\n\t安全 ID:\t\tS-1-5-18\r\n\t帐户名称:\t\tSYSTEM\r\n\t帐户域:\t\tNT AUTHORITY\r\n\t登录 ID:\t\t0x3E7\r\n\t链接的登录 ID:\t\t0x0\r\n\t网络帐户名称:\t-\r\n\t网络帐户域:\t-\r\n\t登录 GUID:\t\t{00000000-0000-0000-0000-000000000000}\r\n\r\n进程信息:\r\n\t进程 ID:\t\t0x134\r\n\t进程名称:\t\tC:\\Windows\\System32\\services.exe\r\n\r\n网络信息:\r\n\t工作站名称:\t-\r\n\t源网络地址:\t-\r\n\t源端口:\t\t-\r\n\r\n详细的身份验证信息:\r\n\t登录进程:\t\tAdvapi  \r\n\t身份验证数据包:\tNegotiate\r\n\t传递的服务:\t-\r\n\t数据包名(仅限 NTLM):\t-\r\n\t密钥长度:\t\t0\r\n\r\n创建登录会话时，将在被访问的计算机上生成此事件。\r\n\r\n“使用者”字段指示本地系统上请求登录的帐户。这通常是一个服务(例如 Server 服务)或本地进程(例如 Winlogon.exe 或 Services.exe)。\r\n\r\n“登录类型”字段指示发生的登录类型。最常见的类型是 2 (交互式)和 3 (网络)。\r\n\r\n“新登录”字段指示新登录是为哪个帐户创建的，即已登录的帐户。\r\n\r\n“网络”字段指示远程登录请求源自哪里。“工作站名称”并非始终可用，并且在某些情况下可能会留空。\r\n\r\n“模拟级别”字段指示登录会话中的进程可以模拟到的程度。\r\n\r\n“身份验证信息”字段提供有关此特定登录请求的详细信息。\r\n\t- “登录 GUID”是可用于将此事件与 KDC 事件关联起来的唯一标识符。\r\n\t-“传递的服务”指示哪些中间服务参与了此登录请求。\r\n\t-“数据包名”指示在 NTLM 协议中使用了哪些子协议。\r\n\t-“密钥长度”指示生成的会话密钥的长度。如果没有请求会话密钥，则此字段将为 0。\"","opcode":"0","processID":"504","providerGuid":"{54849625-5478-4994-a5ba-3e3b0328c30d}","providerName":"Microsoft-Windows-Security-Auditing","severityValue":"AUDIT_SUCCESS","systemTime":"2025-09-12T08:41:30.6774170Z","task":"12544","threadID":"3532","version":"2"}}},"decoder":{"name":"windows_eventchannel"},"id":"1757666491.6033980","input":{"type":"log"},"location":"EventChannel","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Windows Logon Success","firedtimes":8,"gdpr":["IV_32.2"],"gpg13":["7.1","7.2"],"groups":["windows","windows_security","authentication_success"],"hipaa":["164.312.b"],"id":"60106","level":3,"mail":false,"mitre":{"id":["T1078"],"tactic":["Defense Evasion","Persistence","Privilege Escalation","Initial Access"],"technique":["Valid Accounts"]},"nist_800_53":["AU.14","AC.9"],"pci_dss":["10.2.5"],"tsc":["CC6.8","CC7.2","CC7.3"]},"timestamp":"2025-09-12T08:41:31.658+0000"}},{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"oVYUPZkBNAigJEvDKBu1","_score":0,"_source":{"@timestamp":"2025-09-12T08:39:12.964Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"data":{"win":{"eventdata":{"authenticationPackageName":"Negotiate","elevatedToken":"%%1842","impersonationLevel":"%%1833","keyLength":"0","logonGuid":"{00000000-0000-0000-0000-000000000000}","logonProcessName":"Advapi","logonType":"5","processId":"0x134","processName":"C:\\\\Windows\\\\System32\\\\services.exe","subjectDomainName":"WORKGROUP","subjectLogonId":"0x3e7","subjectUserName":"DESKTOP-UOJJ5R1$","subjectUserSid":"S-1-5-18","targetDomainName":"NT AUTHORITY","targetLinkedLogonId":"0x0","targetLogonId":"0x3e7","targetUserName":"SYSTEM","targetUserSid":"S-1-5-18","virtualAccount":"%%1843"},"system":{"channel":"Security","computer":"DESKTOP-UOJJ5R1","eventID":"4624","eventRecordID":"279004","keywords":"0x8020000000000000","level":"0","message":"\"已成功登录帐户。\r\n\r\n使用者:\r\n\t安全 ID:\t\tS-1-5-18\r\n\t帐户名称:\t\tDESKTOP-UOJJ5R1$\r\n\t帐户域:\t\tWORKGROUP\r\n\t登录 ID:\t\t0x3E7\r\n\r\n登录信息:\r\n\t登录类型:\t\t5\r\n\t受限制的管理员模式:\t-\r\n\t虚拟帐户:\t\t否\r\n\t提升的令牌:\t\t是\r\n\r\n模拟级别:\t\t模拟\r\n\r\n新登录:\r\n\t安全 ID:\t\tS-1-5-18\r\n\t帐户名称:\t\tSYSTEM\r\n\t帐户域:\t\tNT AUTHORITY\r\n\t登录 ID:\t\t0x3E7\r\n\t链接的登录 ID:\t\t0x0\r\n\t网络帐户名称:\t-\r\n\t网络帐户域:\t-\r\n\t登录 GUID:\t\t{00000000-0000-0000-0000-000000000000}\r\n\r\n进程信息:\r\n\t进程 ID:\t\t0x134\r\n\t进程名称:\t\tC:\\Windows\\System32\\services.exe\r\n\r\n网络信息:\r\n\t工作站名称:\t-\r\n\t源网络地址:\t-\r\n\t源端口:\t\t-\r\n\r\n详细的身份验证信息:\r\n\t登录进程:\t\tAdvapi  \r\n\t身份验证数据包:\tNegotiate\r\n\t传递的服务:\t-\r\n\t数据包名(仅限 NTLM):\t-\r\n\t密钥长度:\t\t0\r\n\r\n创建登录会话时，将在被访问的计算机上生成此事件。\r\n\r\n“使用者”字段指示本地系统上请求登录的帐户。这通常是一个服务(例如 Server 服务)或本地进程(例如 Winlogon.exe 或 Services.exe)。\r\n\r\n“登录类型”字段指示发生的登录类型。最常见的类型是 2 (交互式)和 3 (网络)。\r\n\r\n“新登录”字段指示新登录是为哪个帐户创建的，即已登录的帐户。\r\n\r\n“网络”字段指示远程登录请求源自哪里。“工作站名称”并非始终可用，并且在某些情况下可能会留空。\r\n\r\n“模拟级别”字段指示登录会话中的进程可以模拟到的程度。\r\n\r\n“身份验证信息”字段提供有关此特定登录请求的详细信息。\r\n\t- “登录 GUID”是可用于将此事件与 KDC 事件关联起来的唯一标识符。\r\n\t-“传递的服务”指示哪些中间服务参与了此登录请求。\r\n\t-“数据包名”指示在 NTLM 协议中使用了哪些子协议。\r\n\t-“密钥长度”指示生成的会话密钥的长度。如果没有请求会话密钥，则此字段将为 0。\"","opcode":"0","processID":"504","providerGuid":"{54849625-5478-4994-a5ba-3e3b0328c30d}","providerName":"Microsoft-Windows-Security-Auditing","severityValue":"AUDIT_SUCCESS","systemTime":"2025-09-12T08:39:11.9738447Z","task":"12544","threadID":"8364","version":"2"}}},"decoder":{"name":"windows_eventchannel"},"id":"1757666352.5978222","input":{"type":"log"},"location":"EventChannel","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Windows Logon Success","firedtimes":7,"gdpr":["IV_32.2"],"gpg13":["7.1","7.2"],"groups":["windows","windows_security","authentication_success"],"hipaa":["164.312.b"],"id":"60106","level":3,"mail":false,"mitre":{"id":["T1078"],"tactic":["Defense Evasion","Persistence","Privilege Escalation","Initial Access"],"technique":["Valid Accounts"]},"nist_800_53":["AU.14","AC.9"],"pci_dss":["10.2.5"],"tsc":["CC6.8","CC7.2","CC7.3"]},"timestamp":"2025-09-12T08:39:12.964+0000"}}]}},"success":true}
```



### 7.4 根据级别获取告警

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/level/5?limit=1"
```

测试结果

```
{"data":{"took":42,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[{"_index":"wazuh-alerts-4.x-2025.09.12","_type":"","_id":"I1YBPZkBNAigJEvDyhEH","_score":0,"_source":{"@timestamp":"2025-09-12T08:19:10.387Z","agent":{"id":"001","ip":"10.7.108.98","name":"hfw"},"decoder":{"name":"syscheck_registry_value_modified"},"full_log":"Registry Value '[x32] HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits\\SecureTimeLow' modified\nMode: scheduled\nChanged attributes: md5,sha1,sha256\nOld md5sum was: '37df1399bcddd966d0956ce90eb7b7d2'\nNew md5sum is : '2d1e51dbeb52faa7dc98f91ef96de4fb'\nOld sha1sum was: '79cea29b60ca4db4bab899737f7b3beb84795e19'\nNew sha1sum is : '7159b8c480fe4be963968b022b81d0c32546e0aa'\nOld sha256sum was: '6747c93988a4f712a45258db63170e9264119fc4bc78aab734ce6381eb78c203'\nNew sha256sum is : 'e377f3d5fc7e41ad08ed5bda001c108a8085b7d322fdb6708e86e637aee509e7'\n","id":"1757665150.5516899","input":{"type":"log"},"location":"syscheck","manager":{"name":"master.instance.cloud.lcpu.dev"},"rule":{"description":"Registry Value Integrity Checksum Changed","firedtimes":9,"gdpr":["II_5.1.f"],"gpg13":["4.13"],"groups":["ossec","syscheck","syscheck_entry_modified","syscheck_registry"],"hipaa":["164.312.c.1","164.312.c.2"],"id":"750","level":5,"mail":false,"mitre":{"id":["T1565.001","T1112"],"tactic":["Impact","Defense Evasion"],"technique":["Stored Data Manipulation","Modify Registry"]},"nist_800_53":["SI.7"],"pci_dss":["11.5"],"tsc":["PI1.4","PI1.5","CC6.1","CC6.8","CC7.2","CC7.3"]},"syscheck":{"arch":"[x32]","changed_attributes":["md5","sha1","sha256"],"event":"modified","md5_after":"2d1e51dbeb52faa7dc98f91ef96de4fb","md5_before":"37df1399bcddd966d0956ce90eb7b7d2","mode":"scheduled","path":"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet\\Services\\W32Time\\SecureTimeLimits","sha1_after":"7159b8c480fe4be963968b022b81d0c32546e0aa","sha1_before":"79cea29b60ca4db4bab899737f7b3beb84795e19","sha256_after":"e377f3d5fc7e41ad08ed5bda001c108a8085b7d322fdb6708e86e637aee509e7","sha256_before":"6747c93988a4f712a45258db63170e9264119fc4bc78aab734ce6381eb78c203","size_after":"8","value_name":"SecureTimeLow","value_type":"REG_QWORD"},"timestamp":"2025-09-12T08:19:10.387+0000"}}]}},"success":true}
```



### 7.5 聚合告警统计

```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/alerts/aggregate" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "terms",
    "field": "rule.level",
    "size": 10
  }'
```

测试结果

```
{"data":{"took":66,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[]},"aggregations":{"group_by":{"buckets":[{"doc_count":400689,"key":3},{"doc_count":121167,"key":5},{"doc_count":16183,"key":7},{"doc_count":5867,"key":10},{"doc_count":399,"key":9},{"doc_count":134,"key":4},{"doc_count":57,"key":12},{"doc_count":36,"key":13},{"doc_count":14,"key":11},{"doc_count":12,"key":8}],"doc_count_error_upper_bound":0,"sum_other_doc_count":0}}},"success":true}
```



### 7.6 获取告警统计

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/stats"
```

测试结果

```
{"data":{"by_agent":{"took":97,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[]},"aggregations":{"group_by":{"buckets":[{"doc_count":394768,"key":"000"},{"doc_count":116087,"key":"010"},{"doc_count":20085,"key":"001"},{"doc_count":10796,"key":"008"},{"doc_count":1480,"key":"006"},{"doc_count":1409,"key":"011"},{"doc_count":71,"key":"007"}],"doc_count_error_upper_bound":0,"sum_other_doc_count":0}}},"by_level":{"took":70,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[]},"aggregations":{"group_by":{"buckets":[{"doc_count":400827,"key":3},{"doc_count":121167,"key":5},{"doc_count":16183,"key":7},{"doc_count":5867,"key":10},{"doc_count":399,"key":9},{"doc_count":134,"key":4},{"doc_count":57,"key":12},{"doc_count":36,"key":13},{"doc_count":14,"key":11},{"doc_count":12,"key":8}],"doc_count_error_upper_bound":0,"sum_other_doc_count":0}}},"by_rule":{"took":81,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[]},"aggregations":{"group_by":{"buckets":[{"doc_count":367665,"key":"80730"},{"doc_count":79715,"key":"5710"},{"doc_count":30266,"key":"5503"},{"doc_count":12583,"key":"550"},{"doc_count":12490,"key":"60106"},{"doc_count":11509,"key":"23502"},{"doc_count":5423,"key":"5760"},{"doc_count":4362,"key":"5551"},{"doc_count":3379,"key":"5402"},{"doc_count":2146,"key":"23508"}],"doc_count_error_upper_bound":13,"sum_other_doc_count":15158}}}},"success":true}
```



---

## 8. 监控和统计 

### 8.1 获取监控概览
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/overview"
```

测试结果

```
{"data":{"agents_summary":{"data":{"connection":{"active":2,"disconnected":2,"never_connected":11,"pending":0,"total":15},"configuration":{"synced":4,"not_synced":11,"total":15}},"message":"","error":0},"cluster_info":{"cluster_name":"wazuh-cluster","cluster_uuid":"JzCS5kS7Rh6H7_A5vrotBQ","name":"node-1","tagline":"The OpenSearch Project: https://opensearch.org/","version":{"build_date":"2025-04-30T10:49:16.411257895Z","build_hash":"dae2bfc93896178873b43cdf4781f183c72b238f","build_snapshot":false,"build_type":"rpm","lucene_version":"9.12.1","minimum_index_compatibility_version":"7.0.0","minimum_wire_compatibility_version":"7.10.0","number":"7.10.2"}},"indexer_health":{"cluster_name":"wazuh-cluster","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":252,"active_shards":252,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":14,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":0,"active_shards_percent_as_number":94.73684210526315},"manager_info":{"data":{"affected_items":[{"path":"/var/ossec","version":"v4.12.0","type":"server","max_agents":"unlimited","openssl_support":"yes","tz_offset":"+0000","tz_name":"UTC"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"Basic information was successfully read","error":0},"manager_status":{"data":{"affected_items":[{"wazuh-agentlessd":"stopped","wazuh-analysisd":"running","wazuh-authd":"running","wazuh-csyslogd":"running","wazuh-dbd":"stopped","wazuh-monitord":"running","wazuh-execd":"running","wazuh-integratord":"stopped","wazuh-logcollector":"running","wazuh-maild":"stopped","wazuh-remoted":"running","wazuh-reportd":"stopped","wazuh-syscheckd":"running","wazuh-clusterd":"stopped","wazuh-modulesd":"running","wazuh-db":"running","wazuh-apid":"running"}],"total_affected_items":1,"total_failed_items":0,"failed_items":[]},"message":"Processes status was successfully read","error":0}},"success":true}%
```



### 8.2 获取Agent摘要

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/agents/summary"
```

测试结果

```
{"data":{"data":{"connection":{"active":2,"disconnected":2,"never_connected":11,"pending":0,"total":15},"configuration":{"synced":4,"not_synced":11,"total":15}},"message":"","error":0},"success":true}
```



### 8.3 获取告警摘要

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/alerts/summary"
```

测试结果

```
{"data":{"took":68,"timed_out":false,"_shards":{"failed":0,"skipped":0,"successful":168,"total":168},"hits":{"total":{"relation":"gte","value":10000},"max_score":0,"hits":[]},"aggregations":{"group_by":{"buckets":[{"doc_count":394788,"key":"000"},{"doc_count":116087,"key":"010"},{"doc_count":20085,"key":"001"},{"doc_count":10796,"key":"008"},{"doc_count":1480,"key":"006"},{"doc_count":1409,"key":"011"},{"doc_count":71,"key":"007"}],"doc_count_error_upper_bound":0,"sum_other_doc_count":0}}},"success":true}
```



### 8.4 获取系统统计

```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/system/stats"
```

测试结果

```
{"data":{"agents_summary":{"data":{"connection":{"active":2,"disconnected":2,"never_connected":11,"pending":0,"total":15},"configuration":{"synced":4,"not_synced":11,"total":15}},"message":"","error":0},"indexer_health":{"cluster_name":"wazuh-cluster","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"active_primary_shards":252,"active_shards":252,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":14,"delayed_unassigned_shards":0,"number_of_pending_tasks":0,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":0,"active_shards_percent_as_number":94.73684210526315},"manager_stats":{"data":{"affected_items":[{"alerts":[{"level":0,"sigid":5521,"times":12},{"level":0,"sigid":5522,"times":12},{"level":0,"sigid":40700,"times":103},{"level":5,"sigid":40704,"times":2},{"level":0,"sigid":80700,"times":77},{"level":3,"sigid":80730,"times":273},{"level":2,"sigid":1002,"times":15},{"level":0,"sigid":530,"times":373},{"level":7,"sigid":531,"times":1},{"level":7,"sigid":533,"times":2},{"level":1,"sigid":535,"times":1},{"level":3,"sigid":502,"times":1},{"level":0,"sigid":515,"times":6},{"level":0,"sigid":221,"times":172},{"level":0,"sigid":19000,"times":4},{"level":0,"sigid":60103,"times":505},{"level":3,"sigid":60106,"times":60},{"level":0,"sigid":63100,"times":2},{"level":0,"sigid":61100,"times":3},{"level":3,"sigid":61104,"times":2},{"level":0,"sigid":60600,"times":3255},{"level":0,"sigid":60609,"times":8},{"level":3,"sigid":60610,"times":4},{"level":3,"sigid":60635,"times":4},{"level":0,"sigid":60640,"times":43},{"level":3,"sigid":60642,"times":6},{"level":3,"sigid":60668,"times":1},{"level":3,"sigid":60669,"times":1},{"level":0,"sigid":60795,"times":1},{"level":5,"sigid":60796,"times":1},{"level":3,"sigid":60798,"times":1},{"level":3,"sigid":60805,"times":1},{"level":5,"sigid":594,"times":12},{"level":5,"sigid":750,"times":13}],"events":7108,"firewall":0,"hour":4,"syscheck":31,"totalAlerts":4977},{"alerts":[{"level":0,"sigid":2830,"times":1},{"level":0,"sigid":5521,"times":2},{"level":0,"sigid":5522,"times":2},{"level":0,"sigid":40700,"times":250},{"level":0,"sigid":80700,"times":196},{"level":3,"sigid":80730,"times":1053},{"level":0,"sigid":530,"times":160},{"level":0,"sigid":221,"times":297},{"level":0,"sigid":60103,"times":84},{"level":3,"sigid":60106,"times":10},{"level":0,"sigid":61100,"times":1},{"level":0,"sigid":60600,"times":653},{"level":0,"sigid":60640,"times":24},{"level":3,"sigid":60642,"times":1}],"events":7270,"firewall":0,"hour":5,"syscheck":0,"totalAlerts":2734},{"alerts":[{"level":0,"sigid":2830,"times":1},{"level":0,"sigid":2900,"times":9},{"level":7,"sigid":2902,"times":2},{"level":7,"sigid":2904,"times":3},{"level":0,"sigid":5521,"times":4},{"level":0,"sigid":5522,"times":4},{"level":0,"sigid":40700,"times":293},{"level":0,"sigid":80700,"times":194},{"level":3,"sigid":80730,"times":1050},{"level":10,"sigid":23505,"times":3},{"level":7,"sigid":23504,"times":8},{"level":5,"sigid":23503,"times":5},{"level":3,"sigid":23502,"times":16},{"level":0,"sigid":530,"times":160},{"level":0,"sigid":221,"times":282},{"level":0,"sigid":60103,"times":76},{"level":3,"sigid":60106,"times":10},{"level":0,"sigid":60600,"times":652}],"events":7291,"firewall":0,"hour":6,"syscheck":0,"totalAlerts":2772},{"alerts":[{"level":0,"sigid":650,"times":1},{"level":3,"sigid":657,"times":1},{"level":0,"sigid":40700,"times":84},{"level":0,"sigid":80700,"times":71},{"level":3,"sigid":80730,"times":356},{"level":2,"sigid":1002,"times":5},{"level":0,"sigid":530,"times":62},{"level":7,"sigid":533,"times":2},{"level":3,"sigid":502,"times":1},{"level":3,"sigid":503,"times":3},{"level":3,"sigid":506,"times":3},{"level":0,"sigid":515,"times":8},{"level":0,"sigid":221,"times":52},{"level":0,"sigid":19000,"times":8},{"level":0,"sigid":60103,"times":64},{"level":3,"sigid":60106,"times":5},{"level":0,"sigid":60600,"times":224}],"events":4732,"firewall":0,"hour":7,"syscheck":8,"totalAlerts":950}],"failed_items":[],"total_affected_items":4,"total_failed_items":0},"error":0,"message":"Statistical information for each node was successfully read"}},"success":true}
```

