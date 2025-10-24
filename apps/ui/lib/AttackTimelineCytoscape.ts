import cytoscape from 'cytoscape';
import { NodeExpansionManager } from './NodeExpansionManager';
import { ThreatAPI } from './threatApi';
import { formatTechniqueDisplay, initializeTechniqueTranslations } from '../utils/techniqueTranslations';

// 节点类型定义 - 更新图标路径
const NODE_TYPES = {
  1: { name: '进程', color: '#4CAF50', borderColor: '#388E3C', icon: '/assets/threat-graph-icons/process-new.svg' },
  2: { name: '网络', color: '#2196F3', borderColor: '#1976D2', icon: '/assets/threat-graph-icons/network-new.svg' },
  3: { name: '文件', color: '#FF9800', borderColor: '#F57C00', icon: '/assets/threat-graph-icons/file-new.svg' },
  4: { name: '注册表', color: '#9C27B0', borderColor: '#7B1FA2', icon: '/assets/threat-graph-icons/registry-new.svg' }
};

// 接口定义
interface GraphNode {
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

interface GraphEdge {
  source: string;
  target: string;
  technique?: string;
  syscall?: string;
  tactic?: string;
  time_stamp?: string;
  edge_desc?: string;
}

interface ParsedGraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export class AttackTimelineCytoscape {
  private cy: any;
  private container: HTMLElement;
  private processedData: ParsedGraphData | null = null;
  private phases: GraphNode[] = [];
  private expandedPhases: Set<string> = new Set();
  private edgeTooltip: HTMLElement | null = null;
  private expansionManager: NodeExpansionManager;
  private currentThreatId: string = '';
  private clickTimeout: any = null;
  private isDestroyed: boolean = false;
  
  // 布局常量
  private readonly PHASE_SPACING = 250;
  private readonly NODE_SPACING = 120;
  private readonly NODE_SIZE = 64;  // 统一节点尺寸，正方形
  private readonly ABSTRACT_NODE_SIZE = 80;  // 抽象节点稍大一些

  constructor(containerId: string, config?: any) {
    this.container = document.getElementById(containerId)!;
    if (!this.container) {
      throw new Error(`Container with id "${containerId}" not found`);
    }
    
    // 初始化展开管理器
    this.expansionManager = new NodeExpansionManager();
    
    // 初始化技术翻译数据
    initializeTechniqueTranslations().catch(err => {
      console.warn('⚠️ [TRANSLATION] 翻译数据初始化失败:', err);
    });
    
    // 确保容器有正确的尺寸
    this.setupContainer();
    this.initializeCytoscape();
  }

  /**
   * 设置容器尺寸
   */
  private setupContainer(): void {
    // 确保容器有正确的尺寸
    if (this.container.offsetWidth === 0 || this.container.offsetHeight === 0) {
      this.container.style.width = '100%';
      this.container.style.height = 'calc(100vh - 200px)';
      this.container.style.minHeight = '600px';
    }
    
    console.log(`📐 [CONTAINER-SETUP] 容器尺寸: ${this.container.offsetWidth}x${this.container.offsetHeight}`);
  }

  /**
   * 初始化Cytoscape实例
   */
  private initializeCytoscape(): void {
    // 清理容器
    this.container.innerHTML = '';
    
    console.log(`📦 [CYTOSCAPE-INIT] 容器信息:`, {
      id: this.container.id,
      width: this.container.offsetWidth,
      height: this.container.offsetHeight,
      clientWidth: this.container.clientWidth,
      clientHeight: this.container.clientHeight
    });

    this.cy = cytoscape({
      container: this.container,
      
      // 样式定义
      style: [
        {
          selector: 'node.abstract-phase',
          style: {
            'background-color': '#0d6efd',
            'border-color': '#0a58ca',
            'border-width': 2,
            'label': 'data(label)',
            'text-valign': 'center',
            'text-halign': 'center',
            'text-wrap': 'wrap',
            'text-max-width': '100px',
            'font-size': '12px',
            'font-weight': 'bold',
            'color': '#ffffff',
            'width': this.ABSTRACT_NODE_SIZE,
            'height': this.ABSTRACT_NODE_SIZE,
            'shape': 'roundrectangle',
            'z-index': 100,
            'overlay-opacity': 0
          }
        },
        {
          selector: 'node.concrete-node',
          style: {
            'background-color': 'data(color)',
            'background-image': 'data(icon)',
            'background-fit': 'none',
            'background-position-x': '50%',
            'background-position-y': '50%',
            'background-width': '50px',
            'background-height': '50px',
            'background-image-opacity': 1,
            'background-image-containment': 'over',
            'background-image-smoothing': 'no',
            'min-zoomed-font-size': '8px',
            'source-distance-normalization': 'none',
            'target-distance-normalization': 'none',
            'border-color': 'data(borderColor)',
            'border-width': 2,
            'label': 'data(label)',
            'text-valign': 'bottom',
            'text-halign': 'center',
            'text-wrap': 'wrap',
            'text-max-width': '80px',
            'text-margin-y': 6,
            'font-size': '10px',
            'color': '#333333',
            'font-weight': 'bold',
            'width': this.NODE_SIZE,
            'height': this.NODE_SIZE,
            'shape': 'roundrectangle',
            'overlay-opacity': 0
          }
        },
        {
          selector: 'edge',
          style: {
            'width': 2,
            'line-color': '#6c757d',
            'target-arrow-color': '#6c757d',
            'target-arrow-shape': 'triangle',
            'curve-style': 'taxi',  // 使用taxi路径（横平竖直）
            'taxi-direction': 'auto',  // 自动选择方向
            'taxi-turn': 20,  // 转弯半径
            'taxi-turn-min-distance': 10,  // 最小转弯距离
            'edge-distances': 'node-position',  // 边偏移基于节点位置
            'segment-distances': 20,  // 多条边之间的间距
            'target-label': 'data(edgeLabel)',  // 标签显示在目标端（箭头处）
            'target-text-offset': 10,  // 离目标节点的距离
            'font-size': '9px',
            'text-rotation': 'none',  // 固定文字方向
            'text-background-color': '#ffffff',
            'text-background-opacity': 0.9,
            'text-background-padding': '3px',
            'text-background-shape': 'roundrectangle',
            'source-endpoint': 'outside-to-node',
            'target-endpoint': 'outside-to-node'
          }
        },
        {
          selector: 'edge.phase-connection',
          style: {
            'line-color': '#0d6efd',
            'width': 3,
            'target-arrow-color': '#0d6efd',
            'target-arrow-shape': 'triangle',
            'curve-style': 'taxi',  // 明确指定
            'taxi-direction': 'horizontal',
            'taxi-turn': 10,
            'z-index': 1,  // 确保在底层
            'target-label': '',  // 不显示标签
            'label': ''  // 清空标签
          }
        },
        {
          selector: 'edge.cross-lane-edge',
          style: {
            'line-color': '#198754',
            'target-arrow-color': '#198754',
            'line-style': 'dashed',
            'taxi-direction': 'horizontal'
          }
        },
        {
          selector: 'edge.same-lane-edge',
          style: {
            'taxi-direction': 'vertical'
          }
        },
        {
          selector: 'edge:selected',
          style: {
            'width': 4,
            'line-color': '#dc3545',
            'target-arrow-color': '#dc3545'
          }
        },
        {
          selector: 'node:selected',
          style: {
            'border-width': 4,
            'border-color': '#ffc107'
          }
        },
        {
          selector: 'node.phase-divider',
          style: {
            'width': 1,
            'height': 8000,  // 再次延长，确保覆盖更大范围
            'background-color': '#6c757d',  // 更深的灰色
            'shape': 'rectangle',
            'border-width': 0,
            'label': '',
            'z-index': -1,  // 确保在最底层
            'opacity': 0.6,  // 提高透明度让线条更明显
            'events': 'no'  // 不响应事件
          }
        }
      ],
      
      // 布局配置
      layout: {
        name: 'preset'  // 手动控制位置
      },
      
      // 交互配置
      wheelSensitivity: 0.1,
      minZoom: 0.3,
      maxZoom: 3,
      boxSelectionEnabled: false,
      autounselectify: false
    });

    // 添加Cytoscape渲染完成检查
    this.cy.ready(() => {
      console.log('✅ [CYTOSCAPE-READY] Cytoscape实例准备完成');
      console.log(`📏 [CYTOSCAPE-READY] 画布尺寸: ${this.cy.width()}x${this.cy.height()}`);
    });

    console.log('🚀 [CYTOSCAPE-INIT] Cytoscape实例初始化完成');
  }

  /**
   * 加载数据
   */
  public loadData(graphData: ParsedGraphData, threatId?: string): void {
    console.log('📊 [LOAD-DATA] 开始加载威胁数据', {
      graphData: graphData,
      hasNodes: !!graphData.nodes,
      hasEdges: !!graphData.edges,
      nodeLength: graphData.nodes?.length || 0,
      edgeLength: graphData.edges?.length || 0,
      threatId: threatId
    });

    // 检查数据结构
    if (!graphData.nodes || !graphData.edges) {
      console.error('❌ [LOAD-DATA] 数据结构不完整:', graphData);
      return;
    }

    this.processedData = graphData;
    this.currentThreatId = threatId || 'unknown';
    
    // 初始化展开管理器
    this.expansionManager.initializeFromGraphData(graphData);
    
    this.processTimelineData();
  }

  /**
   * 处理时间线数据
   */
  private processTimelineData(): void {
    if (!this.processedData) return;

    // 提取抽象节点作为阶段
    this.phases = this.processedData.nodes
      .filter(node => node.node_abstract === "1")
      .sort((a, b) => {
        // 按开始时间排序，如果没有时间信息则按ID排序
        if (a.node_start_time && b.node_start_time) {
          return new Date(a.node_start_time).getTime() - new Date(b.node_start_time).getTime();
        }
        return a.id.localeCompare(b.id);
      });

    // 检查是否有node_source为null的节点
    const nullSourceNodes = this.processedData.nodes.filter(node => 
      node.node_abstract !== "1" && !node.node_source
    );

    if (nullSourceNodes.length > 0) {
      // 创建虚拟的"Unassigned"阶段
      const unassignedPhase: GraphNode = {
        id: 'unassigned',
        node_desc: 'Unassigned',
        node_label: 'Unassigned',
        node_abstract: "1"
      };
      this.phases.push(unassignedPhase);
      
      console.log(`📋 [UNASSIGNED] 发现 ${nullSourceNodes.length} 个未分配节点，创建Unassigned泳道`);
    }

    console.log(`🎯 [TIMELINE-DATA] 提取到 ${this.phases.length} 个攻击阶段:`, 
      this.phases.map(p => ({ id: p.id, desc: p.node_desc, label: p.node_label })));
  }

  /**
   * 渲染时间线
   */
  public render(): void {
    console.log('🎨 [RENDER] 开始渲染攻击时间线');
    
    this.createInitialTimeline();
    this.bindTimelineEvents();
    
    // 适应视图并设置更大的初始缩放
    setTimeout(() => {
      if (this.cy) {
        this.cy.fit();
        this.cy.center();
        // 设置更大的初始缩放级别（调整到2.8倍以适应更长的泳道）
        this.cy.zoom(this.cy.zoom() * 2.8);
        this.cy.center();
      }
    }, 100);

    console.log('✅ [RENDER] 攻击时间线渲染完成');
  }

  /**
   * 创建初始时间线（只显示抽象节点）
   */
  private createInitialTimeline(): void {
    // 清空现有内容
    this.cy.elements().remove();
    this.expandedPhases.clear();

    // 只添加抽象节点
    this.phases.forEach((phase, index) => {
      const x = 100 + index * this.PHASE_SPACING;
      const y = 50;

      this.cy.add({
        data: {
          id: `phase_${phase.id}`,
          originalId: phase.id,
          label: phase.node_name || phase.node_label || `阶段${phase.id}`,
          isAbstract: true,
          phaseIndex: index
        },
        position: { x, y },
        classes: 'abstract-phase',
        locked: true  // 固定位置，防止拖动
      });
    });

    // 添加阶段分割线
    if (this.phases.length > 1) {
      for (let i = 0; i < this.phases.length - 1; i++) {
        const x = 100 + (i + 0.5) * this.PHASE_SPACING;  // 在两个阶段中间
        
        this.cy.add({
          data: {
            id: `divider_${i}`,
            isDivider: true
          },
          position: { x, y: 400 },  // y设为画布中心
          classes: 'phase-divider',
          locked: true,
          selectable: false,
          grabbable: false
        });
      }
      console.log(`📏 [DIVIDERS] 添加了 ${this.phases.length - 1} 条阶段分割线`);
    }

    // 添加阶段间连接
    for (let i = 0; i < this.phases.length - 1; i++) {
      this.cy.add({
        data: {
          id: `phase_edge_${i}`,
          source: `phase_${this.phases[i].id}`,
          target: `phase_${this.phases[i + 1].id}`,
          edgeLabel: '',
          isPhaseConnection: true
        },
        classes: 'phase-connection'
      });
    }

    console.log(`🔗 [INITIAL-TIMELINE] 创建了 ${this.phases.length} 个抽象节点和 ${this.phases.length - 1} 个阶段连接`);
  }

  /**
   * 绑定时间线事件
   */
  private bindTimelineEvents(): void {
    // 点击抽象节点展开/收缩
    this.cy.on('tap', 'node.abstract-phase', (event) => {
      const node = event.target;
      const phaseId = node.data('originalId');
      
      console.log(`🎯 [CLICK] 点击抽象节点: ${phaseId}`);
      
      if (this.expandedPhases.has(phaseId)) {
        this.collapsePhase(phaseId);
      } else {
        this.expandPhase(phaseId);
      }
    });

    // 点击具体节点展开其连接的子节点（延迟处理以区分双击）
    this.cy.on('tap', 'node.concrete-node', (event) => {
      const node = event.target;
      const nodeId = node.data('originalId') || node.id();
      
      console.log(`🎯 [CLICK] 点击具体节点: ${nodeId}`);
      
      // 延迟300ms执行，如果期间有双击则取消
      this.clickTimeout = setTimeout(async () => {
        await this.handleConcreteNodeClick(nodeId);
      }, 300);
    });

    // 双击具体节点显示详情
    this.cy.on('dblclick', 'node.concrete-node', async (event) => {
      event.stopPropagation();
      
      // 取消单击事件
      if (this.clickTimeout) {
        clearTimeout(this.clickTimeout);
        this.clickTimeout = null;
      }
      
      const node = event.target;
      const nodeId = node.data('originalId') || node.id();
      
      console.log(`🔍 [DOUBLE-CLICK] 双击具体节点: ${nodeId}`);
      
      await this.showNodeDetailModal(nodeId);
    });

    // 边的hover事件
    this.cy.on('mouseover', 'edge', (event) => {
      const edge = event.target;
      const data = edge.data();

      // 跳过阶段连接边
      if (data.isPhaseConnection) return;

      if (data.technique || data.syscall || data.tactic || data.time_stamp) {
        this.showEdgeTooltip(data, event.renderedPosition);
      }
    });

    this.cy.on('mouseout', 'edge', () => {
      this.hideEdgeTooltip();
    });

    // 双击边显示详情
    this.cy.on('dblclick', 'edge', (event) => {
      const edge = event.target;
      const data = edge.data();
      
      // 跳过阶段连接边
      if (data.isPhaseConnection) return;
      
      this.showEdgeDetails(data);
    });

    // 节点hover效果
    this.cy.on('mouseover', 'node', (event) => {
      const node = event.target;
      node.style({
        'border-width': 4
      });
    });

    this.cy.on('mouseout', 'node', (event) => {
      const node = event.target;
      if (!node.selected()) {
        node.style({
          'border-width': 2
        });
      }
    });

    // 节点拖拽约束 - 确保节点不能超出所在阶段的分割线范围
    this.cy.on('drag', 'node.concrete-node', (event: any) => {
      const node = event.target;
      const phaseId = node.data('phaseId');
      const phaseIndex = this.phases.findIndex(p => p.id === phaseId);
      
      if (phaseIndex !== -1) {
        const position = node.position();
        
        // 计算阶段边界（基于分割线位置）
        const phaseX = 100 + phaseIndex * this.PHASE_SPACING;
        const leftBoundary = phaseX - this.PHASE_SPACING * 0.4;
        const rightBoundary = phaseX + this.PHASE_SPACING * 0.4;
        
        let newX = position.x;
        if (newX < leftBoundary) {
          newX = leftBoundary;
        } else if (newX > rightBoundary) {
          newX = rightBoundary;
        }
        
        // 如果需要约束，更新节点位置
        if (newX !== position.x) {
          node.position({ x: newX, y: position.y });
        }
      }
    });

    console.log('🎮 [EVENTS] 事件绑定完成');
  }

  /**
   * 展开阶段
   */
  private expandPhase(phaseId: string): void {
    const phase = this.phases.find(p => p.id === phaseId);
    if (!phase) return;

    const phaseIndex = this.phases.indexOf(phase);
    const laneX = 100 + phaseIndex * this.PHASE_SPACING;

    // 获取该阶段的具体节点
    const concreteNodes = this.getConcreteNodesForPhase(phase);

    console.log(`📦 [EXPAND] 展开阶段 ${phaseId}: ${concreteNodes.length} 个具体节点`);

    // 添加具体节点到垂直泳道
    concreteNodes.forEach((node, index) => {
      const y = 150 + index * this.NODE_SPACING;
      const nodeType = this.getNodeTypeInfo(node.node_type || 1);

      this.cy.add({
        data: {
          id: node.id,
          originalId: node.id,
          label: node.node_name || `节点${node.id}`,
          color: nodeType.color,
          borderColor: nodeType.borderColor,
          icon: nodeType.icon,
          node_type: node.node_type,
          phaseId: phaseId
        },
        position: { x: laneX, y },
        classes: 'concrete-node'
      });
    });

    // 添加边（关键：传递所有数据字段）
    this.addEdgesForExpandedNodes(concreteNodes);

    this.expandedPhases.add(phaseId);

    // 应用布局优化
    this.applyLayoutOptimizations();

    console.log(`✅ [EXPAND] 阶段 ${phaseId} 展开完成`);
  }

  /**
   * 收缩阶段
   */
  private collapsePhase(phaseId: string): void {
    console.log(`📤 [COLLAPSE] 收缩阶段: ${phaseId}`);

    // 移除该阶段的所有具体节点
    const nodesToRemove = this.cy.nodes('.concrete-node').filter(node => 
      node.data('phaseId') === phaseId
    );

    // 移除这些节点的所有连接边
    nodesToRemove.forEach(node => {
      node.connectedEdges().remove();
    });

    // 移除节点
    nodesToRemove.remove();

    this.expandedPhases.delete(phaseId);

    console.log(`✅ [COLLAPSE] 阶段 ${phaseId} 收缩完成`);
  }

  /**
   * 计算taxi路径的turn值偏移避免重叠
   */
  private calculateEdgeOffset(source: string, target: string, edgeIndex: number): number {
    // 为taxi路径计算turn值的偏移
    const baseTurn = 20;
    const turnIncrement = 10;
    return baseTurn + (edgeIndex * turnIncrement);
  }

  /**
   * 添加展开节点的边
   */
  private addEdgesForExpandedNodes(nodes: GraphNode[]): void {
    if (!this.processedData) return;

    const visibleNodeIds = new Set(
      this.cy.nodes('.concrete-node').map((n: any) => n.id())
    );

    console.log(`🔗 [ADD-EDGES] 为 ${nodes.length} 个新节点添加边，当前可见节点: ${visibleNodeIds.size}`);

    let addedEdges = 0;

    this.processedData.edges.forEach(edge => {
      // 检查两端节点是否都可见
      if (visibleNodeIds.has(edge.source) && visibleNodeIds.has(edge.target)) {
        const edgeId = `edge_${edge.source}_${edge.target}`;

        // 避免重复添加
        if (this.cy.getElementById(edgeId).length === 0) {
          // 确定边的类型
          const sourceNode = this.cy.getElementById(edge.source);
          const targetNode = this.cy.getElementById(edge.target);
          
          if (sourceNode.length && targetNode.length) {
            const isCrossLane = Math.abs(
              sourceNode.position('x') - targetNode.position('x')
            ) > 100;

            // 不再显示边标签，信息通过弹窗展示
            let edgeLabel = '';

            // 计算边偏移避免重叠
            const edgeOffset = this.calculateEdgeOffset(edge.source, edge.target, addedEdges);
            
            this.cy.add({
              data: {
                id: edgeId,
                source: edge.source,
                target: edge.target,
                // 关键：传递所有原始数据字段
                technique: edge.technique || '',
                syscall: edge.syscall || '',
                tactic: edge.tactic || '',
                time_stamp: edge.time_stamp || '',
                edge_desc: edge.edge_desc || '',
                // 边标签
                edgeLabel: edgeLabel,
                isCrossLane: isCrossLane
              },
              classes: isCrossLane ? 'cross-lane-edge' : 'same-lane-edge',
              style: {
                'taxi-turn': edgeOffset,
                'taxi-direction': isCrossLane ? 'horizontal' : 'vertical'
              }
            });

            addedEdges++;
          }
        }
      }
    });

    console.log(`✅ [ADD-EDGES] 添加了 ${addedEdges} 条边`);
  }

  /**
   * 应用布局优化
   */
  private applyLayoutOptimizations(): void {
    console.log('🎨 [LAYOUT] 应用布局优化');
    
    if (!this.cy) {
      console.warn('⚠️ [LAYOUT] Cytoscape实例为null，跳过布局优化');
      return;
    }
    
    // 由于我们已经在边创建时设置了taxi-direction和taxi-turn，
    // 这里只需要简单的整体布局调整
    
    // 确保画布适合所有元素，并保持更大的缩放级别
    this.cy.fit();
    // 设置更大的缩放级别，保持展开后的可视性（调整到2.8倍以适应更长的泳道）
    this.cy.zoom(this.cy.zoom() * 2.8);
    this.cy.center();
    
    console.log('✅ [LAYOUT] 布局优化完成');
  }

  /**
   * 获取阶段的具体节点（只返回抽象节点的直接出边节点）
   */
  private getConcreteNodesForPhase(phase: GraphNode): GraphNode[] {
    if (!this.processedData) return [];
    
    console.log(`🔍 [GET-NODES-FOR-PHASE] 查找抽象节点 ${phase.id} 的直接出边节点`);
    
    // 直接找抽象节点的出边连接的目标节点
    const targetNodeIds = new Set<string>();
    
    this.processedData.edges.forEach(edge => {
      // 如果源节点是这个抽象节点，记录目标节点
      if (edge.source === phase.id) {
        targetNodeIds.add(edge.target);
        console.log(`✓ [DIRECT-EDGE] 找到出边: ${phase.id} -> ${edge.target}`);
      }
    });
    
    // 返回这些目标节点（排除抽象节点）
    const resultNodes = this.processedData.nodes.filter(node => 
      targetNodeIds.has(node.id) && node.node_abstract !== "1"
    );
    
    console.log(`✅ [GET-NODES-FOR-PHASE] 抽象节点 ${phase.id} 有 ${resultNodes.length} 个直接出边节点:`, 
      resultNodes.map(n => n.id));
    
    return resultNodes;
  }

  /**
   * 获取节点类型信息
   */
  private getNodeTypeInfo(nodeType: number): { 
    name: string; 
    color: string; 
    borderColor: string;
    icon: string;
  } {
    return NODE_TYPES[nodeType] || NODE_TYPES[1];
  }

  /**
   * 显示边的工具提示
   */
  private showEdgeTooltip(edgeData: any, position: { x: number; y: number }): void {
    // 创建tooltip元素
    if (!this.edgeTooltip) {
      this.edgeTooltip = document.createElement('div');
      this.edgeTooltip.className = 'edge-tooltip';
      this.edgeTooltip.style.cssText = `
        position: fixed;
        background: rgba(0, 0, 0, 0.9);
        color: white;
        padding: 8px 12px;
        border-radius: 6px;
        font-size: 12px;
        line-height: 1.4;
        pointer-events: none;
        z-index: 1000;
        box-shadow: 0 2px 8px rgba(0,0,0,0.3);
        max-width: 250px;
        word-wrap: break-word;
        display: none;
      `;
      document.body.appendChild(this.edgeTooltip);
    }

    // 构建tooltip内容
    let content = '';
    if (edgeData.technique) {
      const cleanTechnique = edgeData.technique.replace(/["\s]+/g, ' ').trim();
      content += `<div><strong>技术:</strong> ${formatTechniqueDisplay(edgeData.technique)}</div>`;
    }
    if (edgeData.syscall) content += `<div><strong>系统调用:</strong> ${edgeData.syscall}</div>`;
    if (edgeData.tactic) content += `<div><strong>战术:</strong> ${edgeData.tactic}</div>`;
    if (edgeData.time_stamp) {
      // 转换时间戳为可读格式
      const timestamp = new Date(parseInt(edgeData.time_stamp) / 1000000).toLocaleString();
      content += `<div><strong>时间:</strong> ${timestamp}</div>`;
    }

    if (!content) {
      content = `<div>边连接: ${edgeData.source} → ${edgeData.target}</div>`;
    }

    this.edgeTooltip.innerHTML = content;
    this.edgeTooltip.style.left = `${position.x + 10}px`;
    this.edgeTooltip.style.top = `${position.y - 10}px`;
    this.edgeTooltip.style.display = 'block';
  }

  /**
   * 隐藏边的工具提示
   */
  private hideEdgeTooltip(): void {
    if (this.edgeTooltip) {
      this.edgeTooltip.style.display = 'none';
    }
  }

  /**
   * 显示边的详细信息
   */
  private showEdgeDetails(edgeData: any): void {
    const modal = document.createElement('div');
    modal.className = 'edge-detail-modal';
    modal.style.cssText = `
      position: fixed;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background: rgba(0,0,0,0.5);
      display: flex;
      justify-content: center;
      align-items: center;
      z-index: 2000;
    `;

    const content = document.createElement('div');
    content.style.cssText = `
      background: white;
      padding: 20px;
      border-radius: 8px;
      max-width: 600px;
      width: 90%;
      max-height: 80%;
      overflow-y: auto;
      box-shadow: 0 4px 20px rgba(0,0,0,0.3);
      color: #333;
    `;

    console.log('🔍 [EDGE-DETAIL] 边详细信息:', {
      source: edgeData.source,
      target: edgeData.target,
      technique: edgeData.technique,
      syscall: edgeData.syscall,
      tactic: edgeData.tactic,
      time_stamp: edgeData.time_stamp
    });

    let detailsHtml = '<h3 style="color: #333;">🔗 边连接详情</h3>';
    
    // 获取源和目标节点的详细信息
    const sourceNode = this.cy.getElementById(edgeData.source).data();
    const targetNode = this.cy.getElementById(edgeData.target).data();
    
    // 显示源和目标节点信息，包括阶段
    detailsHtml += `<div style="background: #f8f9fa; padding: 10px; border-radius: 4px; margin-bottom: 15px;">`;
    detailsHtml += `<p style="margin: 0; color: #333;"><strong>📤 源节点:</strong> <code>${edgeData.source}</code></p>`;
    if (sourceNode) {
      detailsHtml += `<p style="margin: 2px 0 0 20px; font-size: 0.9em; color: #666;">节点名: ${sourceNode.label || '未知'}</p>`;
      if (sourceNode.phaseId) {
        detailsHtml += `<p style="margin: 2px 0 0 20px; font-size: 0.9em; color: #666;">所属阶段: ${sourceNode.phaseId}</p>`;
      }
    }
    
    detailsHtml += `<p style="margin: 8px 0 0 0; color: #333;"><strong>📥 目标节点:</strong> <code>${edgeData.target}</code></p>`;
    if (targetNode) {
      detailsHtml += `<p style="margin: 2px 0 0 20px; font-size: 0.9em; color: #666;">节点名: ${targetNode.label || '未知'}</p>`;
      if (targetNode.phaseId) {
        detailsHtml += `<p style="margin: 2px 0 0 20px; font-size: 0.9em; color: #666;">所属阶段: ${targetNode.phaseId}</p>`;
      }
    }
    detailsHtml += `</div>`;
    
    // 显示完整的4个字段信息
    detailsHtml += `<h4 style="color: #333;">📋 边属性信息</h4>`;
    if (edgeData.technique) {
      const techniqueText = formatTechniqueDisplay(edgeData.technique);
      detailsHtml += `<p style="color: #333;"><strong>🎯 攻击技术 (Technique):</strong> <span style="word-wrap: break-word; display: inline-block; max-width: 500px;">${techniqueText}</span></p>`;
    } else {
      detailsHtml += `<p style="color: #333;"><strong>🎯 攻击技术 (Technique):</strong> <span style="color: #666;">无</span></p>`;
    }
    
    if (edgeData.syscall) {
      detailsHtml += `<p style="color: #333;"><strong>⚙️ 系统调用 (Syscall):</strong> ${edgeData.syscall}</p>`;
    } else {
      detailsHtml += `<p style="color: #333;"><strong>⚙️ 系统调用 (Syscall):</strong> <span style="color: #666;">无</span></p>`;
    }
    
    if (edgeData.tactic) {
      detailsHtml += `<p style="color: #333;"><strong>🛡️ 攻击战术 (Tactic):</strong> ${edgeData.tactic}</p>`;
    } else {
      detailsHtml += `<p style="color: #333;"><strong>🛡️ 攻击战术 (Tactic):</strong> <span style="color: #666;">无</span></p>`;
    }
    
    if (edgeData.time_stamp) {
      // 处理时间戳格式
      let formattedTime = edgeData.time_stamp;
      try {
        // 尝试转换纳秒时间戳
        const timestamp = new Date(parseInt(edgeData.time_stamp) / 1000000);
        if (!isNaN(timestamp.getTime())) {
          formattedTime = timestamp.toLocaleString();
        }
      } catch (e) {
        // 如果转换失败，保持原始值
      }
      detailsHtml += `<p style="color: #333;"><strong>⏰ 时间戳 (Time Stamp):</strong> ${formattedTime}</p>`;
    } else {
      detailsHtml += `<p style="color: #333;"><strong>⏰ 时间戳 (Time Stamp):</strong> <span style="color: #666;">无</span></p>`;
    }
    
    // 其他信息
    if (edgeData.edge_desc) {
      detailsHtml += `<h4 style="color: #333;">📝 其他信息</h4>`;
      detailsHtml += `<p style="color: #333;"><strong>描述:</strong> ${edgeData.edge_desc}</p>`;
    }

    detailsHtml += '<button id="close-modal" style="margin-top: 20px; padding: 10px 20px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 14px;">关闭</button>';

    content.innerHTML = detailsHtml;
    modal.appendChild(content);
    document.body.appendChild(modal);

    // 关闭事件
    const closeModal = () => {
      try {
        if (modal && modal.parentNode) {
          modal.parentNode.removeChild(modal);
        }
      } catch (error) {
        console.warn('⚠️ [MODAL] 关闭边详情模态框时出现警告:', error);
      }
    };

    modal.addEventListener('click', (e) => {
      if (e.target === modal) closeModal();
    });

    content.querySelector('#close-modal')?.addEventListener('click', closeModal);
  }

  /**
   * 重置视图
   */
  public resetView(): void {
    console.log('🔄 [RESET] 重置视图');
    
    // 收缩所有展开的阶段
    Array.from(this.expandedPhases).forEach(phaseId => {
      this.collapsePhase(phaseId);
    });
    
    // 重新适应视图并设置更大的缩放
    setTimeout(() => {
      if (this.cy) {
        this.cy.fit();
        this.cy.center();
        // 设置更大的缩放级别（调整到2.8倍以适应更长的泳道）
        this.cy.zoom(this.cy.zoom() * 2.8);
        this.cy.center();
      }
    }, 100);
  }

  /**
   * 适应视图并设置更大的缩放级别
   */
  public fit(): void {
    if (!this.cy) {
      console.warn('⚠️ [FIT] Cytoscape实例为null，无法执行fit操作');
      return;
    }
    this.cy.fit();
    this.cy.center();
    // 设置更大的缩放级别（调整到2.8倍以适应更长的泳道）
    this.cy.zoom(this.cy.zoom() * 2.8);
    this.cy.center();
  }

  /**
   * 处理具体节点点击事件
   */
  private async handleConcreteNodeClick(nodeId: string): Promise<void> {
    // 检查是否有子节点
    if (!this.expansionManager.hasChildren(nodeId)) {
      // 没有子节点，显示详情（可以后续实现）
      console.log(`ℹ️ [CONCRETE-CLICK] 节点 ${nodeId} 没有子节点，显示详情`);
      return;
    }
    
    // 有子节点，执行展开/收缩
    const nodeState = this.expansionManager.getNodeState(nodeId);
    if (!nodeState) return;
    
    if (nodeState.isExpanded) {
      console.log(`📦 [COLLAPSE-CONCRETE] 收缩具体节点: ${nodeId}`);
      this.collapseConcreteNode(nodeId);
    } else {
      console.log(`📦 [EXPAND-CONCRETE] 展开具体节点: ${nodeId}`);
      this.expandNodeByEdges(nodeId);
    }
  }

  /**
   * 基于边展开节点（从ThreatGraphCytoscapeEnhanced复用逻辑）
   */
  private expandNodeByEdges(nodeId: string): void {
    console.log(`🎯 [EXPAND-BY-EDGES] 基于边展开节点: ${nodeId}`);
    
    const result = this.expansionManager.expandNode(nodeId, 1);
    
    console.log(`🎯 [EXPAND-BY-EDGES] 展开结果:`, {
      nodesToShow: result.nodesToShow.size,
      edgesToAdd: result.edgesToAdd.length,
      nodeIds: Array.from(result.nodesToShow)
    });
    
    // 渲染要显示的节点
    result.nodesToShow.forEach(childNodeId => {
      console.log(`🔍 [EXPAND-DEBUG] 查找子节点 ${childNodeId}...`);
      
      // 在具体节点中查找
      const nodeData = this.processedData?.nodes.find(n => n.id === childNodeId);
      
      if (nodeData && nodeData.node_abstract !== "1") {
        console.log(`✓ [EXPAND-DEBUG] 找到具体节点 ${childNodeId}: ${nodeData.node_name || nodeData.node_desc}`);
        
        // 确定子节点应该放在哪个阶段
        const targetPhase = this.determineTargetPhase(nodeData, nodeId);
        const phaseIndex = this.phases.findIndex(p => p.id === targetPhase);
        
        if (phaseIndex !== -1) {
          // 确保目标阶段已展开
          if (!this.expandedPhases.has(targetPhase)) {
            this.expandedPhases.add(targetPhase);
          }
          
          const laneX = 100 + phaseIndex * this.PHASE_SPACING;
          const existingNodesInPhase = this.cy.nodes('.concrete-node').filter(n => 
            n.data('phaseId') === targetPhase
          );
          const y = 150 + existingNodesInPhase.length * this.NODE_SPACING;
          
          const nodeType = this.getNodeTypeInfo(nodeData.node_type || 1);
          
          this.cy.add({
            data: {
              id: nodeData.id,
              originalId: nodeData.id,
              label: nodeData.node_name || `节点${nodeData.id}`,
              color: nodeType.color,
              borderColor: nodeType.borderColor,
              icon: nodeType.icon,
              node_type: nodeData.node_type,
              phaseId: targetPhase
            },
            position: { x: laneX, y },
            classes: 'concrete-node'
          });
        }
      }
    });
    
    // 添加边（延迟以确保节点已渲染）
    setTimeout(() => {
      result.edgesToAdd.forEach(edge => {
        const sourceExists = this.cy.getElementById(edge.source).length > 0;
        const targetExists = this.cy.getElementById(edge.target).length > 0;
        
        if (sourceExists && targetExists) {
          // 🔧 从processedData.edges找完整的边数据
          const fullEdgeData = this.processedData?.edges.find(e => 
            e.source === edge.source && e.target === edge.target
          );
          
          console.log(`🔍 [EDGE-LOOKUP] 查找完整边数据:`, {
            original: edge,
            found: fullEdgeData,
            hasFullData: !!fullEdgeData?.technique
          });
          
          // 使用完整的边数据，如果找不到就用原来的
          this.addEdgeWithData(edge.source, edge.target, fullEdgeData || edge);
        }
      });
    }, 100);
  }

  /**
   * 收缩具体节点
   */
  private collapseConcreteNode(nodeId: string): void {
    console.log(`📦 [COLLAPSE-CONCRETE] 收缩具体节点: ${nodeId}`);
    
    const result = this.expansionManager.collapseNode(nodeId);
    
    // 移除子节点
    result.nodesToHide.forEach(childId => {
      const element = this.cy.getElementById(childId);
      if (element.length > 0) {
        element.connectedEdges().remove();
        element.remove();
      }
    });
  }

  /**
   * 确定子节点应该放在哪个阶段
   */
  private determineTargetPhase(nodeData: GraphNode, parentNodeId: string): string {
    // 子节点的阶段信息
    const childPhase = nodeData.node_source;
    
    if (!childPhase) {
      // 如果没有阶段信息，查找父节点所在的阶段
      const parentNode = this.cy.getElementById(parentNodeId);
      if (parentNode.length > 0) {
        return parentNode.data('phaseId') || this.phases[0]?.id || 'unassigned';
      }
      return this.phases[0]?.id || 'unassigned';
    }
    
    // 查找对应的阶段
    const targetPhase = this.phases.find(p => 
      p.node_label === childPhase || p.node_desc === childPhase
    );
    
    return targetPhase ? targetPhase.id : (this.phases[0]?.id || 'unassigned');
  }

  /**
   * 添加带完整数据的边
   */
  private addEdgeWithData(sourceId: string, targetId: string, edgeData: any): void {
    const edgeId = `edge_${sourceId}_${targetId}`;
    
    // 避免重复添加
    if (this.cy.getElementById(edgeId).length > 0) return;
    
    // 确定边的类型
    const sourceNode = this.cy.getElementById(sourceId);
    const targetNode = this.cy.getElementById(targetId);
    
    if (sourceNode.length && targetNode.length) {
      const isCrossLane = Math.abs(
        sourceNode.position('x') - targetNode.position('x')
      ) > 100;
      
      // 不再显示边标签，详细信息通过点击弹窗展示
      let edgeLabel = '';
      
      // 计算边偏移避免重叠 - 使用当前边的数量作为索引
      const currentEdgeCount = this.cy.edges().length;
      const edgeOffset = this.calculateEdgeOffset(sourceId, targetId, currentEdgeCount);
      
      this.cy.add({
        data: {
          id: edgeId,
          source: sourceId,
          target: targetId,
          // 传递所有边数据
          technique: edgeData.technique || '',
          syscall: edgeData.syscall || '',
          tactic: edgeData.tactic || '',
          time_stamp: edgeData.time_stamp || '',
          edge_desc: edgeData.edge_desc || '',
          edgeLabel: edgeLabel,
          isCrossLane: isCrossLane
        },
        classes: isCrossLane ? 'cross-lane-edge' : 'same-lane-edge',
        style: {
          'taxi-turn': edgeOffset,
          'taxi-direction': isCrossLane ? 'horizontal' : 'vertical'
        }
      });
    }
  }

  /**
   * 显示节点详情模态框
   */
  private async showNodeDetailModal(nodeId: string): Promise<void> {
    if (!this.currentThreatId) {
      console.error('❌ [NODE-DETAIL] 威胁ID未设置');
      return;
    }
    
    console.log(`📋 [NODE-DETAIL] 显示节点详情: ${nodeId}`);
    
    try {
      const nodeInfo = await ThreatAPI.getNodeInfo(this.currentThreatId, nodeId);
      if (nodeInfo && nodeInfo.data) {
        this.showDetailModal('节点详细信息', this.formatNodeInfo(nodeInfo.data));
      } else {
        console.warn(`⚠️ [NODE-DETAIL] 节点 ${nodeId} 信息为空`);
        this.showDetailModal('节点详细信息', '<p>该节点暂无详细信息</p>');
      }
    } catch (error) {
      console.error('❌ [NODE-DETAIL] 获取节点详情失败:', error);
      this.showDetailModal('节点详细信息', '<p style="color: #dc3545;">获取节点详情失败，请稍后重试</p>');
    }
  }

  /**
   * 格式化节点信息
   */
  private formatNodeInfo(data: any): string {
    if (!data) return '<p style="color: #333;">无数据</p>';
    
    let html = '<div class="node-info-grid" style="line-height: 1.6; color: #333;">';
    
    // 基本信息
    html += '<h4 style="color: #007bff; margin-top: 0;">🔧 基本信息</h4>';
    html += `<p style="color: #333;"><strong>当前进程:</strong> ${data.node_current_process || '无'}</p>`;
    html += `<p style="color: #333;"><strong>当前命令:</strong> ${data.node_current_command || '无'}</p>`;
    html += `<p style="color: #333;"><strong>当前阶段:</strong> ${data.node_current_period || '无'}</p>`;
    html += `<p style="color: #333;"><strong>节点分数:</strong> ${data.node_current_score || '无'}</p>`;
    html += `<p style="color: #333;"><strong>攻击技术:</strong> ${data.node_current_tech || '无'}</p>`;
    
    // 父进程信息
    if (data.node_father_name || data.node_faher_command) {
      html += '<h4 style="color: #28a745; margin-top: 20px;">👨‍👦 父进程信息</h4>';
      html += `<p style="color: #333;"><strong>父进程名:</strong> ${data.node_father_name || '无'}</p>`;
      html += `<p style="color: #333;"><strong>父进程命令:</strong> ${data.node_faher_command || '无'}</p>`;
      html += `<p style="color: #333;"><strong>父进程ID:</strong> ${data.node_father_process_num || '无'}</p>`;
    }
    
    // 时间信息
    if (data.node_start_time || data.node_end_time) {
      html += '<h4 style="color: #ffc107; margin-top: 20px;">⏰ 时间信息</h4>';
      const startTime = data.node_start_time === '-1' ? '未知' : data.node_start_time;
      const endTime = data.node_end_time === '-1' ? '未知' : data.node_end_time;
      html += `<p style="color: #333;"><strong>开始时间:</strong> ${startTime || '无'}</p>`;
      html += `<p style="color: #333;"><strong>结束时间:</strong> ${endTime || '无'}</p>`;
    }
    
    // 机器信息
    if (data.node_current_machine_id || data.node_current_machine_name) {
      html += '<h4 style="color: #17a2b8; margin-top: 20px;">💻 机器信息</h4>';
      html += `<p style="color: #333;"><strong>机器ID:</strong> ${data.node_current_machine_id || '无'}</p>`;
      html += `<p style="color: #333;"><strong>机器名称:</strong> ${data.node_current_machine_name || '无'}</p>`;
      html += `<p style="color: #333;"><strong>机器位置:</strong> ${data.node_current_machine_location || '无'}</p>`;
      html += `<p style="color: #333;"><strong>风险等级:</strong> <span style="color: #dc3545; font-weight: bold;">${data.node_current_machine_risk || '无'}</span></p>`;
      html += `<p style="color: #333;"><strong>机器状态:</strong> ${data.node_current_machine_state || '无'}</p>`;
      html += `<p style="color: #333;"><strong>部署情况:</strong> ${data.node_current_machine_Deployment || '无'}</p>`;
    }
    
    // 其他未分类信息（过滤已显示的字段）
    const displayedFields = [
      'node_current_process', 'node_current_command', 'node_current_period', 'node_current_score',
      'node_current_tech', 'node_father_name', 'node_faher_command', 'node_father_process_num',
      'node_start_time', 'node_end_time', 'node_current_machine_id', 'node_current_machine_name',
      'node_current_machine_location', 'node_current_machine_risk', 'node_current_machine_state',
      'node_current_machine_Deployment', 'node_current_process_num'
    ];
    
    const otherFields = Object.keys(data).filter(key => !displayedFields.includes(key));
    if (otherFields.length > 0) {
      html += '<h4 style="color: #6c757d; margin-top: 20px;">📝 其他信息</h4>';
      otherFields.forEach(key => {
        html += `<p style="color: #333;"><strong>${key}:</strong> ${data[key] || '无'}</p>`;
      });
    }
    
    html += '</div>';
    return html;
  }

  /**
   * 通用详情模态框显示方法
   */
  private showDetailModal(title: string, content: string): void {
    const modal = document.createElement('div');
    modal.className = 'node-detail-modal';
    modal.style.cssText = `
      position: fixed;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background: rgba(0,0,0,0.5);
      display: flex;
      justify-content: center;
      align-items: center;
      z-index: 2000;
    `;

    const modalContent = document.createElement('div');
    modalContent.style.cssText = `
      background: white;
      padding: 20px;
      border-radius: 8px;
      max-width: 700px;
      width: 90%;
      max-height: 80%;
      overflow-y: auto;
      box-shadow: 0 4px 20px rgba(0,0,0,0.3);
    `;

    modalContent.innerHTML = `
      <h3 style="margin-top: 0; color: #333;">${title}</h3>
      ${content}
      <button id="close-node-modal" style="margin-top: 20px; padding: 10px 20px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 14px;">关闭</button>
    `;

    modal.appendChild(modalContent);
    document.body.appendChild(modal);

    // 关闭事件
    const closeModal = () => {
      try {
        if (modal && modal.parentNode) {
          modal.parentNode.removeChild(modal);
        }
      } catch (error) {
        console.warn('⚠️ [MODAL] 关闭节点详情模态框时出现警告:', error);
      }
    };

    modal.addEventListener('click', (e) => {
      if (e.target === modal) closeModal();
    });

    modalContent.querySelector('#close-node-modal')?.addEventListener('click', closeModal);
  }

  /**
   * 销毁组件
   */
  public destroy(): void {
    console.log('💥 [DESTROY] 销毁AttackTimelineCytoscape');
    
    // 避免重复销毁
    if (this.isDestroyed) {
      console.warn('⚠️ [DESTROY] 组件已经被销毁，跳过重复销毁');
      return;
    }
    
    try {
      // 标记为已销毁
      this.isDestroyed = true;
      
      // 清理定时器
      if (this.clickTimeout) {
        clearTimeout(this.clickTimeout);
        this.clickTimeout = null;
      }
      
      // 安全地移除tooltip
      if (this.edgeTooltip) {
        try {
          if (this.edgeTooltip.parentNode) {
            this.edgeTooltip.parentNode.removeChild(this.edgeTooltip);
          }
        } catch (tooltipError) {
          console.warn('⚠️ [DESTROY] tooltip移除警告:', tooltipError);
        }
        this.edgeTooltip = null;
      }
      
      // 销毁Cytoscape实例
      if (this.cy) {
        try {
          this.cy.destroy();
        } catch (cyError) {
          console.warn('⚠️ [DESTROY] Cytoscape销毁警告:', cyError);
        }
        this.cy = null;
      }
      
      // 安全地清理容器
      if (this.container) {
        try {
          this.container.innerHTML = '';
        } catch (containerError) {
          console.warn('⚠️ [DESTROY] 容器清理警告:', containerError);
        }
      }
      
      console.log('✅ [DESTROY] AttackTimelineCytoscape销毁完成');
    } catch (error) {
      console.warn('⚠️ [DESTROY] 销毁过程中的警告:', error);
    }
  }
}