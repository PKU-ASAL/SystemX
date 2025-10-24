// 节点展开状态管理器
import { 
  NodeExpansionState, 
  ProcessTreeNode, 
  ExpansionResult,
  GraphNode,
  GraphEdge,
  ParsedGraphData
} from '../types/threatGraph';

export class NodeExpansionManager {
  private expansionStates: Map<string, NodeExpansionState>;
  private processTree: Map<string, ProcessTreeNode>;
  private parentChildMap: Map<string, Set<string>>;
  private childParentMap: Map<string, string>;
  private edgeMap: Map<string, GraphEdge[]>;
  private nodeDataMap: Map<string, GraphNode>;
  private expandedPaths: Set<string>;

  constructor() {
    this.expansionStates = new Map();
    this.processTree = new Map();
    this.parentChildMap = new Map();
    this.childParentMap = new Map();
    this.edgeMap = new Map();
    this.nodeDataMap = new Map();
    this.expandedPaths = new Set();
  }

  /**
   * 初始化：从图数据构建进程树和状态
   */
  initializeFromGraphData(graphData: ParsedGraphData): void {
    console.log('🔄 [ExpansionManager] 初始化节点展开管理器');
    
    // 1. 存储所有节点数据 - 支持hop_sequence格式
    const nodes = graphData.nodes || graphData.hop_sequence || [];
    const edges = graphData.edges || graphData.metadata?.originalEdges || [];
    
    nodes.forEach(node => {
      this.nodeDataMap.set(node.id || node.node_id, node);
    });

    // 2. 构建边的映射关系
    edges.forEach(edge => {
      // 兼容不同的字段名：source/from/src, target/to/dst
      const sourceId = edge.source || edge.from || edge.src;
      const targetId = edge.target || edge.to || edge.dst;
      
      if (!sourceId || !targetId) return;
      
      // 存储父子关系
      if (!this.parentChildMap.has(sourceId)) {
        this.parentChildMap.set(sourceId, new Set());
      }
      this.parentChildMap.get(sourceId)!.add(targetId);
      this.childParentMap.set(targetId, sourceId);

      // 存储边信息
      if (!this.edgeMap.has(sourceId)) {
        this.edgeMap.set(sourceId, []);
      }
      if (!this.edgeMap.has(targetId)) {
        this.edgeMap.set(targetId, []);
      }
      this.edgeMap.get(sourceId)!.push(edge);
      this.edgeMap.get(targetId)!.push(edge);
    });

    // 3. 构建进程树
    this.buildProcessTree(graphData);

    // 4. 初始化所有节点的展开状态
    nodes.forEach(node => {
      this.initializeNodeState(node);
    });

    console.log('✅ [ExpansionManager] 初始化完成', {
      totalNodes: this.nodeDataMap.size,
      totalEdges: edges.length,
      abstractNodes: Array.from(this.nodeDataMap.values()).filter(n => n.node_abstract === "1").length
    });
  }

  /**
   * 检查节点是否有子节点
   */
  hasChildren(nodeId: string): boolean {
    return this.parentChildMap.has(nodeId) && this.parentChildMap.get(nodeId)!.size > 0;
  }

  /**
   * 获取节点的子节点
   */
  getChildren(nodeId: string): ProcessTreeNode[] {
    // 基于边关系获取子节点
    const childIds = this.parentChildMap.get(nodeId) || new Set();
    const children: ProcessTreeNode[] = [];
    const missingNodes: string[] = [];
    
    childIds.forEach(childId => {
      const treeNode = this.processTree.get(childId);
      if (treeNode) {
        children.push(treeNode);
      } else {
        missingNodes.push(childId);
      }
    });
    
    console.log(`📦 [CHILDREN] 节点 ${nodeId} 有 ${children.length} 个直接子节点`);
    if (missingNodes.length > 0) {
      console.warn(`⚠️ [MISSING] 节点 ${nodeId} 有 ${missingNodes.length} 个子节点未在processTree中找到:`, missingNodes);
    }
    
    return children;
  }

  /**
   * 展开节点
   */
  expandNode(nodeId: string, level: number = 1): ExpansionResult {
    const state = this.expansionStates.get(nodeId);
    const result: ExpansionResult = {
      nodesToShow: new Set(),
      nodesToHide: new Set(),
      edgesToAdd: [],
      edgesToRemove: []
    };

    if (!state) return result;

    // 获取要显示的子节点
    const nodesToExpand = this.determineNodesToExpand(nodeId, level, state);
    
    // 更新状态
    nodesToExpand.forEach(childId => {
      result.nodesToShow.add(childId);
      state.expandedChildren.add(childId);
      state.visibleChildren.add(childId);
      
      // 添加边
      result.edgesToAdd.push({
        source: nodeId,
        target: childId,
        type: this.getEdgeType(nodeId, childId)
      });
    });

    state.isExpanded = true;
    state.expandLevel = Math.max(state.expandLevel, level);

    console.log(`📦 [Expand] 展开节点 ${nodeId}`, {
      level,
      nodesToShow: result.nodesToShow.size,
      currentExpandLevel: state.expandLevel
    });

    return result;
  }

  /**
   * 收起节点
   */
  collapseNode(nodeId: string): ExpansionResult {
    const state = this.expansionStates.get(nodeId);
    const result: ExpansionResult = {
      nodesToShow: new Set(),
      nodesToHide: new Set(),
      edgesToAdd: [],
      edgesToRemove: []
    };

    if (!state || !state.isExpanded) return result;

    // 递归收集所有需要隐藏的子节点
    const visited = new Set<string>();
    const collectNodesToHide = (id: string) => {
      // 检测循环引用
      if (visited.has(id)) {
        console.warn(`循环检测: collectNodesToHide(${nodeId}) -> ${id}`);
        return;
      }
      visited.add(id);

      const childState = this.expansionStates.get(id);
      if (childState) {
        childState.visibleChildren.forEach(childId => {
          result.nodesToHide.add(childId);
          result.edgesToRemove.push(`${id}-${childId}`);
          collectNodesToHide(childId);
        });
      }
      
      visited.delete(id); // 回溯时移除，允许在不同分支中重复访问
    };

    collectNodesToHide(nodeId);

    // 更新父节点状态
    state.isExpanded = false;
    state.expandLevel = 0;
    state.visibleChildren.clear();
    state.expandedChildren.clear();
    
    // 重置所有被隐藏子节点的状态
    result.nodesToHide.forEach(childId => {
      const childState = this.expansionStates.get(childId);
      if (childState) {
        // 重置子节点的展开状态
        childState.isExpanded = false;
        childState.expandLevel = 0;
        childState.visibleChildren.clear();
        childState.expandedChildren.clear();
      }
    });

    console.log(`📦 [Collapse] 收起节点 ${nodeId}`, {
      nodesToHide: result.nodesToHide.size
    });

    return result;
  }

  /**
   * 获取节点状态
   */
  getNodeState(nodeId: string): NodeExpansionState | undefined {
    return this.expansionStates.get(nodeId);
  }

  /**
   * 获取节点的所有边
   */
  getNodeEdges(nodeId: string): GraphEdge[] {
    return this.edgeMap.get(nodeId) || [];
  }

  // ========== 私有方法 ==========

  /**
   * 初始化节点状态
   */
  private initializeNodeState(node: GraphNode): void {
    const parentId = this.childParentMap.get(node.id);
    const nodeType = this.getNodeType(node);
    
    const state: NodeExpansionState = {
      nodeId: node.id,
      nodeType: nodeType,
      isExpanded: false,
      expandLevel: 0,
      expandedChildren: new Set(),
      visibleChildren: new Set(),
      depth: this.calculateDepth(node.id),
      treeDepth: parentId ? this.getTreeDepth(parentId) + 1 : 0,
      parentId: parentId,
      ancestorIds: this.getAncestorPath(parentId),
      connections: {
        incoming: new Set(),
        outgoing: new Set(),
        bidirectional: new Set()
      },
      metadata: {
        isExecve: this.checkIfExecve(node),
        processName: node.node_name,
        originalCommand: node.node_desc,
        timestamp: node.node_start_time,
        childrenCount: this.parentChildMap.get(node.id)?.size || 0,
        hasHiddenConnections: false
      }
    };

    this.expansionStates.set(node.id, state);
  }

  /**
   * 构建进程树
   */
  private buildProcessTree(graphData: ParsedGraphData): void {
    console.log('🌳 [BuildTree] 开始构建进程树');

    const nodes = graphData.nodes || graphData.hop_sequence || [];
    nodes.forEach(node => {
      const treeNode: ProcessTreeNode = {
        id: node.id,
        name: node.node_name || node.node_desc || '',
        type: this.getNodeType(node),
        description: node.node_desc || '',
        nodeDesc: node.node_desc || '',
        nodeScore: parseFloat(String(node.node_score) || '0'),
        children: [],
        childrenCount: 0,
        processChildren: [],
        fileChildren: [],
        networkChildren: [],
        rawData: node,
        metadata: {
          command: node.node_desc,
          score: parseFloat(String(node.node_score) || '0'),
          stage: node.node_source,
          isRootProcess: this.isRootProcess(node),
          hasExecve: false
        }
      };

      this.processTree.set(node.id, treeNode);
    });

    // 建立树形关系
    this.parentChildMap.forEach((childIds, parentId) => {
      const parentNode = this.processTree.get(parentId);
      if (parentNode) {
        childIds.forEach(childId => {
          const childNode = this.processTree.get(childId);
          if (childNode) {
            parentNode.children.push(childNode);
            parentNode.childrenCount++;
            
            // 分类子节点
            switch (childNode.type) {
              case 'process':
                parentNode.processChildren.push(childId);
                break;
              case 'file':
                parentNode.fileChildren.push(childId);
                break;
              case 'network':
                parentNode.networkChildren.push(childId);
                break;
            }
          }
        });
      }
    });
  }

  /**
   * 获取节点类型
   */
  private getNodeType(node: GraphNode): 'abstract' | 'process' | 'file' | 'network' {
    if (node.node_abstract === "1") return 'abstract';
    
    switch (node.node_type) {
      case 1: return 'process';
      case 2: return 'network';
      case 3: return 'file';
      default: return 'process';
    }
  }

  /**
   * 检查是否为根进程
   */
  private isRootProcess(node: GraphNode): boolean {
    // 抽象节点不是根进程
    if (node.node_abstract === "1") return false;
    
    // 没有父节点的进程节点可能是根进程
    const parentId = this.childParentMap.get(node.id);
    if (!parentId) return true;
    
    // 父节点是抽象节点的进程是根进程
    const parentNode = this.nodeDataMap.get(parentId);
    return parentNode?.node_abstract === "1";
  }

  /**
   * 检查是否为execve节点
   */
  private checkIfExecve(node: GraphNode): boolean {
    if (!node.node_desc) return false;

    const execvePatterns = [
      /exec/i,
      /\/bin\//,
      /\/usr\/bin\//,
      /sh\s+-c/,
      /bash\s+-c/,
      /shell\s+-c/
    ];

    return execvePatterns.some(pattern => pattern.test(node.node_desc!));
  }

  /**
   * 计算节点深度
   */
  private calculateDepth(nodeId: string): number {
    let depth = 0;
    let currentId = nodeId;
    const visited = new Set<string>();
    
    while (this.childParentMap.has(currentId)) {
      if (visited.has(currentId)) {
        console.warn(`循环检测: calculateDepth(${nodeId}) -> ${currentId}`);
        break;
      }
      visited.add(currentId);
      depth++;
      currentId = this.childParentMap.get(currentId)!;
      
      if (depth > 100) break;
    }
    
    return depth;
  }

  /**
   * 获取树深度
   */
  private getTreeDepth(nodeId: string): number {
    const state = this.expansionStates.get(nodeId);
    return state?.treeDepth || 0;
  }

  /**
   * 获取祖先路径
   */
  private getAncestorPath(parentId?: string): string[] {
    if (!parentId) return [];
    
    const ancestors: string[] = [];
    let currentId = parentId;
    const visited = new Set<string>();
    
    while (currentId) {
      if (visited.has(currentId)) {
        console.warn(`祖先路径循环: getAncestorPath(${parentId}) -> ${currentId}`);
        break;
      }
      visited.add(currentId);
      ancestors.unshift(currentId);
      currentId = this.childParentMap.get(currentId);
      
      if (ancestors.length > 50) break;
    }
    
    return ancestors;
  }

  /**
   * 确定要展开的节点
   */
  private determineNodesToExpand(nodeId: string, level: number, state: NodeExpansionState): string[] {
    const nodesToExpand: string[] = [];
    const nodeData = this.nodeDataMap.get(nodeId);
    
    if (!nodeData) return nodesToExpand;

    // 根据节点类型确定展开策略
    if (state.nodeType === 'abstract') {
      // 抽象节点：显示所有第一层子节点
      const children = this.getChildren(nodeId);
      console.log(`📦 [ABSTRACT-EXPAND] 抽象节点 ${nodeId} 的所有子节点:`, {
        totalChildren: children.length,
        types: children.map(c => ({ id: c.id, type: c.type }))
      });
      
      return children.map(child => child.id);
    } else {
      // 具体节点：展开直接子节点
      const children = this.getChildren(nodeId);
      
      console.log(`📦 [CONCRETE-EXPAND] 具体节点 ${nodeId} 的所有子节点:`, {
        totalChildren: children.length,
        childrenIds: children.map(c => c.id)
      });
      
      const filteredChildren = children.filter(child => {
        const isAlreadyExpanded = state.expandedChildren?.has(child.id);
        if (isAlreadyExpanded) {
          console.log(`⚠️ [FILTER] 节点 ${child.id} 已经在expandedChildren中，跳过`);
        }
        return !isAlreadyExpanded;
      });
      
      console.log(`📦 [CONCRETE-EXPAND] 过滤后剩余 ${filteredChildren.length} 个节点`);
      
      return filteredChildren.map(child => child.id);
    }
  }

  /**
   * 获取边类型
   */
  private getEdgeType(sourceId: string, targetId: string): string {
    const sourceNode = this.nodeDataMap.get(sourceId);
    const targetNode = this.nodeDataMap.get(targetId);
    
    if (!sourceNode || !targetNode) return 'concrete-edge';
    
    if (sourceNode.node_abstract === "1" && targetNode.node_abstract === "1") {
      return 'abstract-edge';
    }
    
    if (sourceNode.node_abstract === "0" && targetNode.node_abstract === "0") {
      return 'concrete-edge';
    }
    
    return 'cross-group-edge';
  }
}