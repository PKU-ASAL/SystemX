// 威胁图谱数据类型定义

export interface APIResponse<T = any> {
  code: string;
  message: string;
  data: T;
}

export interface NodeInfoAPIResponse<T = any> {
  code: number;
  msg: string;
  data: T;
}

export interface ThreatNode {
  id: string;
  node_desc?: string;
  node_name?: string;
  node_label?: string;
  node_type?: string;
  node_score?: string;
  node_source?: string;
  node_abstract?: string | boolean;
  description?: string;
  name?: string;
  label?: string;
  type?: string;
  score?: string;
  source?: string;
  is_abstract?: boolean;
  [key: string]: any;
}

export interface ThreatEdge {
  from?: string;
  to?: string;
  source?: string;
  target?: string;
  src?: string;
  dst?: string;
  // 新API支持的边标签字段
  technique?: string;      // 攻击技术标签（显示在边上）
  syscall?: string;        // 系统调用
  tactic?: string;         // 战术标签
  timestamp?: string;      // 时间戳
  [key: string]: any;
}

export interface ProcessedHop {
  hop_id: number;
  depth: number;
  path: string;
  node_id: string;
  node_desc: string;
  node_name?: string;
  node_label?: string;
  node_type?: string;
  node_score?: string;
  node_source?: string;
  is_abstract: boolean;
  timestamps: string[];
  network_connections: NetworkConnection[];
  children_count: number;
  originalNode?: ThreatNode;
}

export interface NetworkConnection {
  source_ip: string;
  source_port: number;
  dest_ip: string;
  dest_port: number;
  connection_string: string;
}

export interface NodeInfo {
  code: number;
  msg: string;
  data: {
    node_start_time?: string;
    node_end_time?: string;
    node_current_score?: string;
    node_current_tech?: string;
    node_current_period?: string;
    node_current_process?: string;
    node_current_process_num?: string;
    node_current_command?: string;
    node_father_name?: string;
    node_faher_command?: string;
    node_father_process_num?: string;
    node_current_machine_id?: string;
    node_current_machine_name?: string;
    node_current_machine_location?: string;
    node_current_machine_risk?: string;
    node_current_machine_state?: string;
    node_current_machine_Deployment?: string;
    [key: string]: any;
  };
}

export interface ThreatGraphData {
  threat_id: string;
  hop_sequence: ProcessedHop[];
  // 添加兼容字段以支持AttackTimelineCytoscape
  nodes?: ThreatNode[];
  edges?: ThreatEdge[];
  max_depth: number;
  bfs_analysis: {
    total_hops: number;
    depth_distribution: { [depth: number]: number };
    timestamped_hops: number;
    non_timestamped_hops: number;
    first_layer_nodes: string[];
  };
  timeline_data: TimelineEntry[];
  network_topology: NetworkTopology;
  metadata: {
    created_at?: string;
    updated_at?: string;
    status?: string;
    severity?: string;
    originalEdges?: ThreatEdge[];
    [key: string]: any;
  };
}

export interface TimelineEntry {
  hop_id: number;
  depth: number;
  timestamp_type: 'extracted' | 'sequence';
  timestamps: string[];
  description: string;
  node_type: string;
  node_label: string;
  network_connections: NetworkConnection[];
  is_abstract: boolean;
}

export interface NetworkTopology {
  nodes: NetworkNode[];
  edges: NetworkEdge[];
  stats: {
    total_nodes: number;
    total_edges: number;
    internal_nodes: number;
    external_nodes: number;
  };
}

export interface NetworkNode {
  ip: string;
  type: 'internal' | 'external';
  connections_out: number;
  connections_in: number;
}

export interface NetworkEdge {
  source: string;
  target: string;
  source_port: number;
  target_port: number;
  hop_id: number;
  // 新增边标签支持
  technique?: string;      // 攻击技术标签
  syscall?: string;        // 系统调用
  tactic?: string;         // 战术标签
  timestamp?: string;      // 时间戳
}

// D3可视化相关类型
export interface D3Node {
  id: string;
  name: string;
  group: number;
  depth: number;
  type: string;
  score?: string;
  description: string;
  isAbstract: boolean;
  x?: number;
  y?: number;
  fx?: number | null;
  fy?: number | null;
  expanded?: boolean;
  isFirstLayer?: boolean;
  isExpandable?: boolean;
  originalData: ProcessedHop;
  detailInfo?: NodeInfo | null;
}

export interface D3Link {
  source: string | D3Node;
  target: string | D3Node;
  value: number;
  // 边标签支持
  technique?: string;      // 攻击技术标签（主要显示）
  syscall?: string;        // 系统调用
  tactic?: string;         // 战术标签
  timestamp?: string;      // 时间戳
}

export interface VisualizationConfig {
  width: number;
  height: number;
  forceStrength: number;
  linkDistance: number;
  chargeStrength: number;
  nodeRadius: number;
  showLabels: boolean;
  enablePhysics: boolean;
  layout: 'force' | 'hierarchical' | 'radial';
}