// 威胁图谱相关类型定义

export interface NodeExpansionState {
  nodeId: string;
  nodeType: 'abstract' | 'process' | 'file' | 'network';
  isExpanded: boolean;
  expandLevel: number;
  expandedChildren: Set<string>;
  visibleChildren: Set<string>;
  depth: number;
  treeDepth: number;
  parentId?: string;
  ancestorIds: string[];
  connections: {
    incoming: Set<string>;
    outgoing: Set<string>;
    bidirectional: Set<string>;
  };
  metadata: {
    isExecve: boolean;
    processName?: string;
    originalCommand?: string;
    timestamp?: string;
    childrenCount: number;
    hasHiddenConnections: boolean;
  };
}

export interface ProcessTreeNode {
  id: string;
  name: string;
  type: 'abstract' | 'process' | 'file' | 'network';
  description?: string;
  nodeDesc: string;
  nodeScore: number;
  children: ProcessTreeNode[];
  childrenCount: number;
  processChildren: string[];
  fileChildren: string[];
  networkChildren: string[];
  rawData: any;
  metadata: {
    command?: string;
    score: number;
    stage?: string;
    isRootProcess: boolean;
    hasExecve: boolean;
  };
}

export interface ExpansionResult {
  nodesToShow: Set<string>;
  nodesToHide: Set<string>;
  edgesToAdd: any[];
  edgesToRemove: any[];
}

// 图节点和边的基础类型
export interface GraphNode {
  id: string;
  node_desc?: string;
  node_name?: string;
  node_label?: string;
  node_abstract?: string;
  node_source?: string;
  node_type?: number;
  node_start_time?: string;
  node_score?: number;
}

export interface GraphEdge {
  source: string;
  target: string;
  technique?: string;
  syscall?: string;
  tactic?: string;
  time_stamp?: string;
  edge_desc?: string;
}

export interface ParsedGraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}