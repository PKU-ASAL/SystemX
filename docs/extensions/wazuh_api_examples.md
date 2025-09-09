# SysArmor Wazuh API 测试示例


---

## 1. 配置管理

### 1.1 获取Wazuh配置
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/config"
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

### 1.4 重新加载配置 
```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/config/reload"
```

---

## 2. Manager管理

### 2.1 获取Manager信息
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/info"
```

### 2.2 获取Manager状态
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/status"
```

### 2.3 获取Manager日志 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/logs"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/logs?limit=5&level=info&search=syscollector"
```

### 2.4 获取Manager统计信息
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/stats"
```

### 2.5 重启Manager
```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/manager/restart"
```

### 2.6 获取Manager配置
```bash
# 获取所有配置
curl -X GET "http://localhost:8080/api/v1/wazuh/manager/configuration"

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


#### 3.1.2 添加新Agent 
```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/agents" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-agent-006",
    "ip": "192.168.1.100"
  }'
```

#### 3.1.3 获取单个Agent详情 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001"
```

#### 3.1.4 删除Agent 
```bash
curl -X DELETE "http://localhost:8080/api/v1/wazuh/agents/017"
```

#### 3.1.5 重启Agent 
```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/agents/001/restart"
```

#### 3.1.6 获取Agent密钥 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/key"
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


### 3.2 Agent详细信息 

#### 3.2.1 获取Agent系统信息 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/system"
```


#### 3.2.2 获取Agent硬件信息 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/hardware"
```

#### 3.2.3 获取Agent端口信息 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/ports"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/ports?limit=10&offset=0&sort=local.port&search=22"
```

#### 3.2.4 获取Agent软件包信息 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/packages"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/packages?limit=5&search=nginx&sort=name"
```

#### 3.2.5 获取Agent进程信息 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/processes"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/processes?limit=10&search=nginx&sort=pid"
```


#### 3.2.6 获取Agent网络协议信息 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netproto"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netproto?limit=10&sort=iface"
```

#### 3.2.7 获取Agent网络地址信息 
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netaddr"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/netaddr?limit=10&search=eth0"
```


### 3.3 Agent统计信息 

#### 3.3.1 获取Agent日志收集器统计 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/stats/logcollector"
```

#### 3.3.2 获取Agent守护进程统计 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/daemons/stats"
```

### 3.4 Agent安全扫描 

#### 3.4.1 获取Agent CIS-CAT扫描结果 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/ciscat"
```

#### 3.4.2 获取Agent SCA扫描结果 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/sca"
```

#### 3.4.3 获取Agent Rootcheck扫描结果 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/rootcheck"
```

#### 3.4.4 清除Agent Rootcheck扫描结果 
```bash
curl -X DELETE "http://localhost:8080/api/v1/wazuh/agents/001/rootcheck"
```

#### 3.4.5 获取Agent Rootcheck最后扫描时间 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/agents/001/rootcheck/last_scan"
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

#### 3.5.4 运行Rootcheck扫描 
```bash
curl -X PUT "http://localhost:8080/api/v1/wazuh/rootcheck" \
  -H "Content-Type: application/json" \
  -d '{
    "agents_list": ["001"]
  }'
```

### 3.6 集群和概览 

#### 3.7.1 获取集群健康状态 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/cluster/health"
```

#### 3.7.2 获取Agent概览信息 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/overview/agents"
```
---

## 4. 组管理

### 4.1 获取组列表
```bash
# 基础查询
curl -X GET "http://localhost:8080/api/v1/wazuh/groups"

# 带参数查询
curl -X GET "http://localhost:8080/api/v1/wazuh/groups?limit=5&sort=name&search=default"
```

### 4.2 创建新组
```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/groups" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web-servers"
  }'
```

### 4.3 获取单个组信息
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/groups/default"
```

### 4.4 获取组内Agent列表
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/groups/default/agents"
```

### 4.5 添加Agent到组
```bash
curl -X POST "http://localhost:8080/api/v1/wazuh/groups/web-servers/agents" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "001"
  }'
```

### 4.6 从组中移除Agent
```bash
curl -X DELETE "http://localhost:8080/api/v1/wazuh/groups/web-servers/agents/001"
```

### 4.7 获取组配置
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/groups/default/configuration"
```

### 4.8 更新组配置
```bash
curl -X PUT "http://localhost:8080/api/v1/wazuh/groups/default/configuration" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_config": {
      "syscheck": {
        "frequency": "43200"
      }
    }
  }'
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

### 5.2 获取单个规则
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/rules/5715"
```



## 6. Indexer管理

### 6.1 获取Indexer健康状态
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/health"
```

### 8.2 获取Indexer信息
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/info"
```

### 8.3 获取索引列表
```bash
# 获取所有索引
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/indices"

# 获取特定模式的索引
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/indices?pattern=wazuh-alerts-*"
```


### 8.4获取索引模板
```bash
# 获取所有模板
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/templates"

# 获取特定模板
curl -X GET "http://localhost:8080/api/v1/wazuh/indexer/templates?pattern=wazuh"
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

### 7.2 根据Agent获取告警 
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/agent/001?limit=2"
```

### 7.3 根据规则获取告警
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/rule/60106?limit=5"
```

### 7.4 根据级别获取告警
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/level/5?limit=10"
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

### 7.6 获取告警统计
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/alerts/stats"
```

---

## 8. 监控和统计 

### 8.1 获取监控概览
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/overview"
```

### 8.2 获取Agent摘要
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/agents/summary"
```

### 8.3 获取告警摘要
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/alerts/summary"
```

### 8.4 获取系统统计
```bash
curl -X GET "http://localhost:8080/api/v1/wazuh/monitoring/system/stats"
```

