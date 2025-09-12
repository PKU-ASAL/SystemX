# SysArmor 事件数据结构设计

## 📋 概述

SysArmor系统中涉及多种事件格式，从原始数据收集到最终的威胁检测，每个阶段都有特定的数据结构。

## 🔄 数据流和事件结构

### 1. **原始数据层** - `sysarmor.raw.*`

#### **Vector输出格式** (来自rsyslog)
```json
{
  "collector_id": "b1de298c-38bd-479d-be94-459778086446",
  "timestamp": "2025-09-11T10:21:12Z",
  "host": "racknerd-915f21b",
  "source": "auditd",
  "message": "type=SYSCALL msg=audit(1757066946.683:1312217): arch=c000003e syscall=42 success=no exit=-1 ppid=24710 pid=31362 comm=\"sshd\" exe=\"/usr/sbin/sshd\"",
  "event_type": "syslog",
  "severity": "info",
  "data_source": "syslog",
  "event_category": "raw_audit",
  "partition_key": "b1de298c-38bd-479d-be94-459778086446",
  "target_topic": "sysarmor.raw.audit",
  "source_type": "socket",
  "port": 52684,
  "processed_at": "2025-09-11T10:21:12.589987845Z",
  "tags": ["audit", "syscall", "network"]
}
```

**特点**：
- **来源**：Vector从rsyslog接收的原始auditd数据
- **message字段**：包含完整的auditd日志行
- **collector_id**：标识数据来源的collector
- **用途**：作为后续处理的输入数据

---

### 2. **结构化事件层** - `sysarmor.events.*`

#### **Flink NODLINK处理器输出格式**
```json
{
  // SysArmor元数据层
  "event_id": "1312217",
  "timestamp": "2025-09-05T11:36:13.299Z",
  "collector_id": "b1de298c-38bd-479d-be94-459778086446",
  "host": "racknerd-915f21b",
  "source": "auditd",
  "processor": "flink-nodlink-converter",
  "processed_at": "2025-09-12T01:04:12.303Z",
  
  // 事件分类（便于快速过滤）
  "event_type": "connect",
  "event_category": "net",
  "severity": "low",
  
  // 完整的NODLINK标准sysdig数据
  "message": {
    "evt.num": 1312217,
    "evt.time": 1757066946.683,
    "evt.type": "connect",
    "evt.category": "net",
    "evt.dir": ">",
    "evt.args": "arch=c000003e a0=8 a1=7f28080037c0 a2=10 a3=5 items=0 auid=4294967295",
    "proc.name": "sshd",
    "proc.exe": "/usr/sbin/sshd",
    "proc.cmdline": "/usr/sbin/sshd -D",
    "proc.pid": 31362,
    "proc.ppid": 24710,
    "proc.pcmdline": "/usr/sbin/sshd -D",
    "proc.uid": 0,
    "proc.gid": 0,
    "fd.name": "",
    "net.sockaddr": {
      "family": "AF_INET",
      "type": "ipv4",
      "source_ip": "45.135.232.92",
      "source_port": 22,
      "address": "45.135.232.92:22"
    },
    "host": "racknerd-915f21b",
    "is_warn": false
  }
}
```

**特点**：
- **来源**：Flink NODLINK处理器处理原始auditd数据
- **双层结构**：外层SysArmor元数据 + 内层标准sysdig数据
- **collector_id保留**：完整的溯源信息
- **NODLINK兼容**：message字段包含标准sysdig格式
- **用途**：威胁检测、异常分析的输入数据

---

### 3. **告警事件层** - `sysarmor.alerts.*`

#### **预期的告警格式** (待实现)
```json
{
  // SysArmor告警元数据
  "alert_id": "alert-1757066946-001",
  "timestamp": "2025-09-12T01:04:15.123Z",
  "collector_id": "b1de298c-38bd-479d-be94-459778086446",
  "host": "racknerd-915f21b",
  "source": "nodlink-detector",
  "processor": "flink-anomaly-detector",
  "processed_at": "2025-09-12T01:04:15.123Z",
  
  // 告警分类
  "alert_type": "anomaly_detection",
  "alert_category": "network_anomaly",
  "severity": "high",
  "confidence": 0.85,
  
  // 告警详情
  "alert": {
    "title": "Suspicious Network Connection",
    "description": "Detected anomalous network connection pattern",
    "risk_score": 85,
    "evidence": [
      {
        "event_id": "1312217",
        "event_type": "connect",
        "anomaly_score": 0.85,
        "reason": "Unusual connection pattern detected"
      }
    ],
    "mitigation": "Block connection from source IP",
    "references": ["MITRE ATT&CK T1071"]
  },
  
  // 原始事件引用
  "source_events": [
    {
      "event_id": "1312217",
      "topic": "sysarmor.events.audit",
      "partition": 17,
      "offset": 3
    }
  ]
}
```

**特点**：
- **来源**：异常检测算法（如NODLINK）处理结构化事件
- **告警元数据**：完整的告警上下文信息
- **证据链**：包含导致告警的原始事件
- **用途**：安全运营中心(SOC)的告警处理

---

## 🎯 事件类型分类

### **按数据处理阶段分类**：

#### **Raw Data (原始数据)**：
- **sysarmor.raw.audit**：auditd原始日志
- **sysarmor.raw.other**：Vector解析失败的数据

#### **Structured Events (结构化事件)**：
- **sysarmor.events.audit**：auditd转换的sysdig格式事件
- **sysarmor.events.sysdig**：Sysdig直接发送的事件

#### **Alerts (告警事件)**：
- **sysarmor.alerts**：一般告警事件
- **sysarmor.alerts.high**：高危告警事件

### **按事件内容分类**：

#### **系统调用事件** (基于NODLINK标准)：
- **文件操作**：read, write, open, openat, chmod, rename, unlink
- **进程操作**：execve, fork, clone, pipe
- **网络操作**：socket, connect, accept, sendto, recvfrom
- **其他操作**：22种NODLINK标准事件类型

#### **安全事件类别**：
- **file**：文件系统操作
- **process**：进程管理操作
- **net**：网络连接操作
- **system**：系统级操作

---

## 🔧 数据结构设计原则

### **1. 分层设计**：
- **外层**：SysArmor管理元数据（collector_id, timestamp, host等）
- **内层**：标准格式数据（sysdig, 告警详情等）

### **2. 向后兼容**：
- **字段命名**：保持与现有API的兼容性
- **数据格式**：支持多种数据源和处理器

### **3. 可扩展性**：
- **模块化设计**：每个处理阶段独立的数据结构
- **标准兼容**：支持NODLINK、Sysdig等标准格式

### **4. 溯源能力**：
- **完整链路**：从collector到最终告警的完整数据链
- **元数据保留**：处理时间、处理器、数据来源等信息

---

## 📊 当前实现状态

### ✅ **已实现**：
1. **原始数据收集**：Vector → sysarmor.raw.audit
2. **结构化转换**：Flink → sysarmor.events.audit
3. **API查询**：Manager API支持多种事件格式查询

### 🔧 **待实现**：
1. **异常检测**：NODLINK算法 → sysarmor.alerts.*
2. **告警管理**：告警聚合、去重、通知
3. **威胁情报**：外部威胁情报集成

---

## 💡 设计优势

### **1. 清晰的数据分层**：
- 每个处理阶段都有明确的输入输出格式
- 便于调试和数据质量监控

### **2. 完整的溯源链**：
- 从collector到告警的完整数据链路
- 支持事件溯源和根因分析

### **3. 标准兼容性**：
- 支持NODLINK、Sysdig等业界标准
- 便于集成第三方工具和算法

### **4. API友好**：
- 统一的查询接口
- 支持多维度过滤和搜索

这种设计为SysArmor提供了灵活、可扩展、标准兼容的事件处理架构。
